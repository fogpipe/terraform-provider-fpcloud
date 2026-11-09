package provider_test

import (
	"context"
	"testing"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/provider"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
)

// An apply waits for each deploy's rollout unless the configuration says not to,
// so whatever the same apply does next runs against the new version.
func TestAppWaitsForRolloutByDefault(t *testing.T) {
	var resp fwresource.SchemaResponse
	provider.NewAppResource().Schema(context.Background(), fwresource.SchemaRequest{}, &resp)
	attr, ok := resp.Schema.Attributes["wait_for_rollout"].(schema.BoolAttribute)
	if !ok {
		t.Fatal("wait_for_rollout missing or not a BoolAttribute")
	}
	if attr.Default == nil {
		t.Fatal("wait_for_rollout has no default")
	}
	var got defaults.BoolResponse
	attr.Default.DefaultBool(context.Background(), defaults.BoolRequest{}, &got)
	if !got.PlanValue.ValueBool() {
		t.Errorf("wait_for_rollout defaults to %v, want true", got.PlanValue)
	}
}
