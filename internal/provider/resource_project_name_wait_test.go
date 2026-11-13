package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/provider"
)

// projectStub answers a project create with refusals first, then the project,
// and a delete with the project in `deleting` that never goes away: a teardown
// still running when the destroy ends.
type projectStub struct {
	mu       sync.Mutex
	refusals []map[string]string
	creates  int
	deleted  bool
	project  client.Project
}

func (s *projectStub) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
		s.creates++
		if s.creates <= len(s.refusals) {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": s.refusals[s.creates-1]})
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(s.project)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/"+s.project.ID:
		p := s.project
		if s.deleted {
			p.Status = "deleting"
		}
		_ = json.NewEncoder(w).Encode(p)
	case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/projects/"+s.project.ID:
		s.deleted = true
		p := s.project
		p.Status = "deleting"
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(p)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newProjectStub(t *testing.T, refusals ...map[string]string) *projectStub {
	t.Helper()
	stamp := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	stub := &projectStub{refusals: refusals, project: client.Project{
		ID: "proj-1", OrganizationID: "org-1", Name: "shop", DisplayName: "shop",
		Status: "active", Namespace: "yqw-shop", Egress: "restricted", CreatedAt: stamp, UpdatedAt: stamp,
	}}
	srv := httptest.NewServer(http.HandlerFunc(stub.serve))
	t.Cleanup(srv.Close)
	t.Setenv("FPCLOUD_API_URL", srv.URL)
	t.Setenv("FPCLOUD_API_KEY", "fp-test")
	return stub
}

var tearingDown = map[string]string{
	"code":    "PROJECT_TEARING_DOWN",
	"message": "a deleted project of this name is still being torn down; retry once its delete has finished",
}

// A destroy returns once the delete is accepted, so a replace under the same
// name reaches the create while the old project's teardown is still running.
// The create waits for the name instead of failing the apply, and the destroy
// does not wait for a teardown nobody needs finished.
func TestProject_CreateWaitsForANameStillBeingTornDown(t *testing.T) {
	terraformBinary(t)
	provider.SetProjectNameFreePoll(t, 10*time.Millisecond)
	stub := newProjectStub(t, tearingDown, tearingDown)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `resource "fpcloud_project" "shop" { name = "shop" }`,
			Check:  resource.TestCheckResourceAttr("fpcloud_project.shop", "id", "proj-1"),
		}},
		// The stub never lets the project go: the destroy has to have returned
		// on the accepted delete for the test to end.
		CheckDestroy: func(*terraform.State) error { return nil },
	})

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.creates != 3 {
		t.Fatalf("the create is retried while the name is held: got %d creates, want 3", stub.creates)
	}
	if !stub.deleted {
		t.Fatal("the destroy never deleted the project")
	}
}

// Only the teardown's refusal is waited out. A name another, live project holds
// is a conflict nothing will resolve, and it fails the create at once.
func TestProject_CreateDoesNotWaitOnAnOrdinaryConflict(t *testing.T) {
	terraformBinary(t)
	provider.SetProjectNameFreePoll(t, 10*time.Millisecond)
	stub := newProjectStub(t, map[string]string{"code": "CONFLICT", "message": "project already exists"})

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      `resource "fpcloud_project" "shop" { name = "shop" }`,
			ExpectError: regexp.MustCompile(`project already exists`),
		}},
	})

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.creates != 1 {
		t.Fatalf("an ordinary conflict is not retried: got %d creates, want 1", stub.creates)
	}
}
