package provider

import (
	"testing"
	"time"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestSetModelFromAppAfterApply_KeepsPlannedValues is a regression test for the
// "Provider produced inconsistent result after apply" failures on every app
// update. The deploy response answered a configured image tag with the resolved
// digest and a configured replica count with a rollout-transient one; blindly
// mapping those into state broke the plugin protocol and failed applies whose
// deploys had in fact succeeded.
func TestSetModelFromAppAfterApply_KeepsPlannedValues(t *testing.T) {
	r := &AppResource{}
	plan := AppResourceModel{
		ID:                  types.StringValue("app-123"),
		ProjectID:           types.StringValue("proj-1"),
		Name:                types.StringValue("api"),
		DisplayName:         types.StringValue("API"),
		Image:               types.StringValue("registry.example.com/acme/api:1950f03"),
		Ingress:             types.StringValue("all"),
		Mode:                types.StringValue("always-on"),
		Storage:             types.StringValue(""),
		StoragePath:         types.StringValue(""),
		Replicas:            types.Int64Value(1),
		MinScale:            types.Int64Value(1),
		MaxScale:            types.Int64Value(10),
		CPULimit:            types.StringValue("500m"),
		MemoryLimit:         types.StringValue("512Mi"),
		HealthCheckPath:     types.StringValue("/"),
		HealthCheckTimeout:  types.Int64Value(5),
		HealthCheckInterval: types.Int64Value(10),
		HealthCheckRetries:  types.Int64Value(3),
		// Computed-only attributes are unknown in the plan and must come from the API.
		Status:    types.StringUnknown(),
		URL:       types.StringUnknown(),
		CreatedAt: types.StringUnknown(),
		UpdatedAt: types.StringUnknown(),
	}

	created := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	r.setModelFromAppAfterApply(&plan, &client.App{
		ID:          "app-123",
		ProjectID:   "proj-1",
		Name:        "api",
		DisplayName: "API",
		// The API answers with a digest and a mid-rollout replica count.
		Image:               "registry.example.com/acme/api@sha256:a9bd393dc52b",
		Replicas:            0,
		Status:              "deploying",
		URL:                 "https://api-acme-fp.app.cloud.fogpipe.com",
		Ingress:             "all",
		Mode:                "always-on",
		MinScale:            1,
		MaxScale:            10,
		CPULimit:            "500m",
		MemoryLimit:         "512Mi",
		HealthCheckPath:     "/",
		HealthCheckTimeout:  5,
		HealthCheckInterval: 10,
		HealthCheckRetries:  3,
		CreatedAt:           created,
		UpdatedAt:           created,
	})

	if got := plan.Image.ValueString(); got != "registry.example.com/acme/api:1950f03" {
		t.Errorf("image = %q, want the configured tag (a digest here fails the apply)", got)
	}
	if got := plan.Replicas.ValueInt64(); got != 1 {
		t.Errorf("replicas = %d, want the planned 1 (a transient 0 here fails the apply)", got)
	}
	if got := plan.Status.ValueString(); got != "deploying" {
		t.Errorf("status = %q, want the API value for an unknown-in-plan attribute", got)
	}
	if plan.URL.IsUnknown() {
		t.Error("url stayed unknown; unknown attributes must be filled from the API")
	}
	if plan.CreatedAt.IsUnknown() || plan.UpdatedAt.IsUnknown() {
		t.Error("timestamps stayed unknown; unknown attributes must be filled from the API")
	}
}

// TestSetModelFromApp_ReportsAPIValues guards the other half of the split: Read
// must keep reporting what the API says so real drift still surfaces as a diff.
func TestSetModelFromApp_ReportsAPIValues(t *testing.T) {
	r := &AppResource{}
	state := AppResourceModel{
		Image:    types.StringValue("registry.example.com/acme/api:old"),
		Replicas: types.Int64Value(1),
	}

	r.setModelFromApp(&state, &client.App{
		Image:    "registry.example.com/acme/api:new",
		Replicas: 3,
	})

	if got := state.Image.ValueString(); got != "registry.example.com/acme/api:new" {
		t.Errorf("image = %q, want the API value so drift is detected on read", got)
	}
	if got := state.Replicas.ValueInt64(); got != 3 {
		t.Errorf("replicas = %d, want the API value so drift is detected on read", got)
	}
}
