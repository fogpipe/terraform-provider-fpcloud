package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/provider"
)

// databaseStub answers a database create with refusals first, then the
// database, and a delete with the database in `deleting` that never goes away:
// a teardown still running when the destroy ends.
type databaseStub struct {
	mu       sync.Mutex
	refusals int
	creates  int
	deleted  bool
	database client.Database
}

func (s *databaseStub) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/proj-1/databases":
		s.creates++
		if s.creates <= s.refusals {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{
				"code":    "DATABASE_TEARING_DOWN",
				"message": "a deleted database of this name is still being torn down; retry once its delete has finished",
			}})
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(s.database)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/databases/"+s.database.ID:
		d := s.database
		if s.deleted {
			d.Status = "deleting"
		}
		_ = json.NewEncoder(w).Encode(d)
	case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/databases/"+s.database.ID:
		s.deleted = true
		d := s.database
		d.Status = "deleting"
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(d)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// A destroy returns once the delete is accepted, so a replace under the same
// name reaches the create while the old database's teardown is still running.
// The create waits for the name instead of failing the apply, and the destroy
// does not wait for a teardown nobody needs finished.
func TestDatabase_CreateWaitsForANameStillBeingTornDown(t *testing.T) {
	terraformBinary(t)
	provider.SetNameFreePoll(t, 10*time.Millisecond)
	stamp := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	stub := &databaseStub{refusals: 2, database: client.Database{
		ID: "db-1", ProjectID: "proj-1", Name: "maindb", DisplayName: "maindb",
		Engine: "postgres", Version: "18", Status: "running", Host: "shop-maindb-rw", Port: 5432,
		CPU: "250m", Memory: "512Mi", Storage: "10Gi", Instances: 2,
		CreatedAt: stamp, UpdatedAt: stamp,
	}}
	srv := httptest.NewServer(http.HandlerFunc(stub.serve))
	t.Cleanup(srv.Close)
	t.Setenv("FPCLOUD_API_URL", srv.URL)
	t.Setenv("FPCLOUD_API_KEY", "fp-test")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `resource "fpcloud_database" "main" {
  project    = "proj-1"
  name       = "maindb"
}`,
			Check: resource.TestCheckResourceAttr("fpcloud_database.main", "id", "db-1"),
		}},
		// The stub never lets the database go: the destroy has to have
		// returned on the accepted delete for the test to end.
		CheckDestroy: func(*terraform.State) error { return nil },
	})

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.creates != 3 {
		t.Fatalf("the create is retried while the name is held: got %d creates, want 3", stub.creates)
	}
	if !stub.deleted {
		t.Fatal("the destroy never deleted the database")
	}
}
