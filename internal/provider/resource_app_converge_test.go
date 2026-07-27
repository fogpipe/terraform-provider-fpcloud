package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestConverge_AdoptedAppIsPushedToConfig covers adoption taking ownership: an
// app that already exists with a different image and no config must be driven
// onto the configuration by the adopting apply, not merely recorded in state.
func TestConverge_AdoptedAppIsPushedToConfig(t *testing.T) {
	var calls []string
	var deployedImage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		if req.URL.Path == "/api/v1/apps/app-1/deploy" {
			var body client.DeployRequest
			_ = json.NewDecoder(req.Body).Decode(&body)
			deployedImage = body.Image
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.App{
			ID:       "app-1",
			Name:     "api",
			Image:    "registry.example.com/acme/api:new",
			Replicas: 2,
			Mode:     "always-on",
		})
	}))
	defer srv.Close()

	r := &AppResource{client: client.New(srv.URL, "test-key")}

	// What the API reports about the app being adopted.
	var adopted AppResourceModel
	r.setModelFromApp(&adopted, &client.App{
		ID:       "app-1",
		Name:     "api",
		Image:    "registry.example.com/acme/api:old",
		Replicas: 2,
		Mode:     "always-on",
	})

	plan := adopted
	plan.Image = types.StringValue("registry.example.com/acme/api:new")

	var diags diag.Diagnostics
	app := r.converge(context.Background(), &plan, adopted, "app-1", &diags)
	if diags.HasError() {
		t.Fatalf("converge returned diagnostics: %v", diags.Errors())
	}
	if app == nil {
		t.Fatal("converge returned no app")
	}
	if deployedImage != "registry.example.com/acme/api:new" {
		t.Errorf("deployed image = %q, want the configured image pushed on adoption", deployedImage)
	}
	if len(calls) == 0 || calls[0] != "POST /api/v1/apps/app-1/deploy" {
		t.Errorf("calls = %v, want a deploy of the configured image first", calls)
	}
}

// TestConverge_NoChangesOnlyReads guards the other direction: when the plan
// already matches, converge issues no mutations and just re-reads the app.
func TestConverge_NoChangesOnlyReads(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.App{ID: "app-1", Image: "img:1", Replicas: 1})
	}))
	defer srv.Close()

	r := &AppResource{client: client.New(srv.URL, "test-key")}

	var current AppResourceModel
	r.setModelFromApp(&current, &client.App{ID: "app-1", Image: "img:1", Replicas: 1, Mode: "always-on"})
	plan := current

	var diags diag.Diagnostics
	if app := r.converge(context.Background(), &plan, current, "app-1", &diags); app == nil {
		t.Fatalf("converge returned no app: %v", diags.Errors())
	}
	if len(calls) != 1 || calls[0] != "GET /api/v1/apps/app-1" {
		t.Errorf("calls = %v, want a single read and no mutations", calls)
	}
}
