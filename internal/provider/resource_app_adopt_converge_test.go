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
)

// adoptStub is an API holding one app named web that its create refuses with
// ALREADY_EXISTS, so a config naming it with adopt_existing adopts it.
type adoptStub struct {
	mu      sync.Mutex
	app     client.App
	config  map[string]string
	changes []client.UpdateConfigRequest
	deleted bool
}

func (s *adoptStub) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/proj-1":
		_ = json.NewEncoder(w).Encode(client.Project{ID: "proj-1", OrganizationID: "org-1", Name: "proj", Status: "active"})
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/proj-1/apps":
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"ALREADY_EXISTS","message":"an app named web already exists"}}`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/proj-1/apps":
		_ = json.NewEncoder(w).Encode([]client.App{s.app})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/app-1":
		if s.deleted {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(s.app)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/app-1/config":
		entries := []client.AppConfig{}
		for k, v := range s.config {
			entries = append(entries, client.AppConfig{AppID: "app-1", Key: k, Value: v})
		}
		_ = json.NewEncoder(w).Encode(entries)
	case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/apps/app-1/config":
		var req client.UpdateConfigRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.changes = append(s.changes, req)
		for _, e := range req.Set {
			s.config[e.Key] = e.Value
		}
		for _, k := range req.Unset {
			delete(s.config, k)
		}
		_ = json.NewEncoder(w).Encode(client.UpdateConfigResponse{})
	case r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/app-1/probes":
		var req client.UpdateProbesRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.app.Probes = req.Probes
		_ = json.NewEncoder(w).Encode(s.app)
	case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/apps/app-1":
		s.deleted = true
		w.WriteHeader(http.StatusAccepted)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newAdoptStub(t *testing.T) *adoptStub {
	t.Helper()
	stamp := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	s := &adoptStub{
		app: client.App{ID: "app-1", ProjectID: "proj-1", Name: "web", DisplayName: "web", Image: "ghcr.io/acme/web:v1",
			Type: "web", Port: 8080, Ingress: "internal", Status: "running", MinScale: 1, MaxScale: 1,
			CPULimit: "500m", MemoryLimit: "512Mi", CreatedAt: stamp, UpdatedAt: stamp},
		config: map[string]string{"FOO": "old", "STALE": "1"},
	}
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)
	t.Setenv("FPCLOUD_API_URL", srv.URL)
	t.Setenv("FPCLOUD_API_KEY", "fp-test")
	return s
}

// An adopted app that differs from its configuration is brought to it in the
// same apply, as an update would, so the apply never ends on state the plan
// did not promise.
func TestAppAdopt_ConvergesAnAppThatDiffers(t *testing.T) {
	terraformBinary(t)
	stub := newAdoptStub(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
resource "fpcloud_app" "web" {
  project        = "proj-1"
  name           = "web"
  image          = "ghcr.io/acme/web:v1"
  adopt_existing = true
  env            = { FOO = "new" }
  probes         = { readiness = { path = "/ready" } }
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("fpcloud_app.web", "id", "app-1"),
				resource.TestCheckResourceAttr("fpcloud_app.web", "probes.readiness.path", "/ready"),
				resource.TestCheckResourceAttr("fpcloud_app.web", "env.FOO", "new"),
			),
		}},
	})

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.changes) != 1 {
		t.Fatalf("want the env converged in one change, got %d", len(stub.changes))
	}
	if got := stub.changes[0].Unset; len(got) != 1 || got[0] != "STALE" {
		t.Errorf("want STALE unset, the configuration not declaring it, got %v", got)
	}
}

// What only a create sets cannot be converged, so an existing app differing
// there is refused by name rather than recorded under a plan it contradicts.
func TestAppAdopt_RefusesWhatOnlyACreateSets(t *testing.T) {
	terraformBinary(t)
	newAdoptStub(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
resource "fpcloud_app" "web" {
  project        = "proj-1"
  name           = "web"
  image          = "ghcr.io/acme/web:v1"
  adopt_existing = true
  ingress        = "all"
}`,
			ExpectError: regexp.MustCompile(`ingress is "internal", the configuration says "all"`),
		}},
	})
}
