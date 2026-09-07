package provider_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

var errReleaseCommandNotRestored = errors.New("apply did not send the configured release command back to the API")

// A release command cleared outside Terraform is drift, and the apply restores
// it. It read as nothing: the read-back kept the prior value whenever the API
// answered with no command, so a config of ["migrate"] and an app that had
// been told to run nothing agreed on every plan, and every deploy from then
// on skipped the migration while reporting success
// (fogpipe/cloud-workspace#348).
func TestAppReleaseCommand_ClearedOutOfBandIsDriftAndIsRestored(t *testing.T) {
	terraformBinary(t)

	var mu sync.Mutex
	cleared := false
	var restoredTo [][]string
	app := client.App{ID: "app-1", ProjectID: "proj-1", Name: "web", Image: "ghcr.io/acme/web:v1",
		Mode: "always-on", Type: "web", Port: 8080, Ingress: "internal", Status: "running", Replicas: 1,
		ReleaseCommand: []string{"migrate"}, CPULimit: "500m", MemoryLimit: "512Mi",
		HealthCheckPath: "/", HealthCheckTimeout: 5, HealthCheckInterval: 10, HealthCheckRetries: 3}
	current := func() client.App {
		a := app
		if cleared {
			a.ReleaseCommand = nil
		}
		return a
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/proj-1/apps":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(app)
		case r.URL.Path == "/api/v1/apps/app-1/deployments":
			_ = json.NewEncoder(w).Encode([]client.Deployment{{ID: "dep-1", AppID: "app-1", Image: app.Image, Status: "active"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/app-1":
			_ = json.NewEncoder(w).Encode(current())
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/app-1/scale":
			_ = json.NewEncoder(w).Encode(current())
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/app-1/command":
			body, _ := io.ReadAll(r.Body)
			var req client.UpdateCommandRequest
			_ = json.Unmarshal(body, &req)
			if req.ReleaseCommand != nil {
				restoredTo = append(restoredTo, *req.ReleaseCommand)
				cleared = len(*req.ReleaseCommand) == 0
			}
			_ = json.NewEncoder(w).Encode(current())
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/apps/app-1":
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
	clearOutOfBand := func() {
		mu.Lock()
		defer mu.Unlock()
		cleared = true
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config},
			{
				// `fpcloud app update web --release-command ''` happened meanwhile.
				PreConfig:          clearOutOfBand,
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.web", "release_command.0", "migrate"),
					func(*terraform.State) error {
						mu.Lock()
						defer mu.Unlock()
						for _, sent := range restoredTo {
							if len(sent) == 1 && sent[0] == "migrate" {
								return nil
							}
						}
						return errReleaseCommandNotRestored
					},
				),
			},
		},
	})
}
