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
)

// An adopting apply — an `import` block beside a config that changes something
// — updates the resource in the same run, and every computed attribute has to
// be known when the apply ends. The update paths that wrote the plan's unknowns
// to state failed exactly there, on `updated_at`, and the second apply then
// passed, so the defect read as a flake (fogpipe/cloud-workspace#257). The
// stub answers every read with timestamps, as the API does.
func TestAppImport_AnAdoptingApplyLeavesNoTimestampUnknown(t *testing.T) {
	terraformBinary(t)

	var mu sync.Mutex
	stamp := time.Date(2026, 9, 2, 17, 0, 0, 0, time.UTC)
	app := client.App{ID: "app-1", ProjectID: "proj-1", Name: "web", DisplayName: "Web", Image: "ghcr.io/acme/web:v1",
		Mode: "always-on", Type: "web", Port: 8080, Ingress: "internal", Status: "running", Replicas: 1,
		CPULimit: "500m", MemoryLimit: "512Mi",
		HealthCheckPath: "/", HealthCheckTimeout: 5, HealthCheckInterval: 10, HealthCheckRetries: 3,
		CreatedAt: stamp, UpdatedAt: stamp}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/app-1":
			_ = json.NewEncoder(w).Encode(app)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/app-1/traffic":
			// The traffic read fails on this apply, as it can mid-rollout. It is
			// the one computed attribute Update did not settle when the API
			// would not answer, and the unknown it left is the same error as
			// the timestamps once produced.
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to get traffic"})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/apps/app-1":
			var req client.UpdateAppRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			app.DisplayName = req.DisplayName
			app.UpdatedAt = stamp.Add(time.Minute)
			_ = json.NewEncoder(w).Encode(app)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/apps/app-1":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	t.Setenv("FPCLOUD_API_URL", srv.URL)
	t.Setenv("FPCLOUD_API_KEY", "fp-test")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
import {
  to = fpcloud_app.web
  id = "app-1"
}

resource "fpcloud_app" "web" {
  project_id   = "proj-1"
  name         = "web"
  display_name = "Web, adopted"
  image        = "ghcr.io/acme/web:v1"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.web", "display_name", "Web, adopted"),
					resource.TestCheckResourceAttr("fpcloud_app.web", "updated_at", stamp.Add(time.Minute).String()),
					resource.TestCheckResourceAttr("fpcloud_app.web", "created_at", stamp.String()),
					resource.TestCheckNoResourceAttr("fpcloud_app.web", "traffic.0.revision"),
				),
			},
		},
	})
}

func TestProjectImport_AnAdoptingApplyLeavesNoTimestampUnknown(t *testing.T) {
	terraformBinary(t)

	var mu sync.Mutex
	stamp := time.Date(2026, 9, 2, 17, 0, 0, 0, time.UTC)
	project := client.Project{ID: "proj-1", OrganizationID: "org-1", Name: "errors", DisplayName: "errors",
		Status: "active", Namespace: "fp-errors", CreatedAt: stamp, UpdatedAt: stamp}
	deleted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/proj-1":
			if deleted {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(project)
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/projects/proj-1":
			var req client.UpdateProjectRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.DisplayName != "" {
				project.DisplayName = req.DisplayName
			}
			project.UpdatedAt = stamp.Add(time.Minute)
			_ = json.NewEncoder(w).Encode(project)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/projects/proj-1":
			deleted = true
			_ = json.NewEncoder(w).Encode(project)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	t.Setenv("FPCLOUD_API_URL", srv.URL)
	t.Setenv("FPCLOUD_API_KEY", "fp-test")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
import {
  to = fpcloud_project.errors
  id = "proj-1"
}

resource "fpcloud_project" "errors" {
  name         = "errors"
  display_name = "Error tracker"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_project.errors", "display_name", "Error tracker"),
					resource.TestCheckResourceAttr("fpcloud_project.errors", "updated_at", stamp.Add(time.Minute).String()),
				),
			},
		},
	})
}
