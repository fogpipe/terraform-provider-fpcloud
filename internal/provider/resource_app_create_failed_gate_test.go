package provider_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"testing"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

var errNotDestroyed = errors.New("the failed app was not destroyed before the retry: Terraform did not record it")

// terraformBinary points terraform-plugin-testing at a CLI when the
// environment has one; a unit test that drives the framework needs it, and
// declines when it is absent rather than pretending to cover this.
func terraformBinary(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC_TERRAFORM_PATH") == "" {
		for _, name := range []string{"terraform", "tofu"} {
			if p, err := exec.LookPath(name); err == nil {
				t.Setenv("TF_ACC_TERRAFORM_PATH", p)
				if name == "tofu" && os.Getenv("TF_ACC_PROVIDER_HOST") == "" {
					t.Setenv("TF_ACC_PROVIDER_HOST", "registry.opentofu.org")
				}
				break
			}
		}
	}
	if os.Getenv("TF_ACC_TERRAFORM_PATH") == "" {
		t.Skip("set TF_ACC_TERRAFORM_PATH (or put terraform/tofu on PATH) to run framework-level tests")
	}
}

// An app created with a release command is a remote object the moment the
// API answers 201, whether or not the gate it is then waited on passes. A
// Create that reported the gate's failure without saving the resource left
// Terraform knowing nothing about an app it had made: the next apply collided
// with a 409 on a name it did not know it owned, and nothing would ever
// destroy the failed app (fogpipe/cloud-workspace#224). Saved and tainted, the
// failed app is Terraform's to replace.
func TestAppCreate_FailedReleaseGateIsStillRecorded(t *testing.T) {
	terraformBinary(t)

	var mu sync.Mutex
	var deleted []string
	created := false
	creates := 0
	// What the API echoes back for a create that states nothing but the
	// defaults, so the second apply's result is consistent with its plan.
	app := client.App{ID: "app-1", ProjectID: "proj-1", Name: "web", Image: "ghcr.io/acme/web:v1",
		Mode: "always-on", Type: "web", Port: 8080, Ingress: "internal", Status: "failed", Replicas: 1,
		ReleaseCommand: []string{"migrate"}, CPULimit: "500m", MemoryLimit: "512Mi",
		HealthCheckPath: "/", HealthCheckTimeout: 5, HealthCheckInterval: 10, HealthCheckRetries: 3}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/proj-1/apps":
			if created {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "app already exists"})
				return
			}
			created = true
			creates++
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(app)
		case r.URL.Path == "/api/v1/apps/app-1/deployments":
			// The gate fails on the first create and passes on the retry.
			status := "failed"
			if creates > 1 {
				status = "active"
			}
			_ = json.NewEncoder(w).Encode([]client.Deployment{{ID: "dep-1", AppID: "app-1", Image: app.Image, Status: status}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/app-1":
			if !created {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(app)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/app-1/scale":
			_ = json.NewEncoder(w).Encode(app)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/apps/app-1":
			deleted = append(deleted, r.URL.Path)
			created = false
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	t.Setenv("FPCLOUD_API_URL", srv.URL)
	t.Setenv("FPCLOUD_API_KEY", "fp-test")

	config := `
resource "fpcloud_app" "web" {
  project_id      = "proj-1"
  name            = "web"
  image           = "ghcr.io/acme/web:v1"
  release_command = ["migrate"]
}`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile("release command failed"),
			},
			{
				// The retry replaces the failed app rather than colliding with it:
				// Terraform destroys what it recorded and creates again.
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.web", "id", "app-1"),
					func(*terraform.State) error {
						mu.Lock()
						defer mu.Unlock()
						if len(deleted) != 1 {
							return errNotDestroyed
						}
						return nil
					},
				),
			},
		},
	})
}
