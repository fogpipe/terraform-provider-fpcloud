package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// A state written while a reference attribute held the id the API answered
// upgrades to that id in both the reference and its computed id, so the
// configuration that wrote it, rewritten onto the new attribute, plans no
// change. Each upgrade has to produce a state the current schema reads: an
// attribute left behind under its old name is refused by the framework, and
// one left out reads as null, which a refresh would repair and a plan without
// one would not.
func TestReferenceStateUpgrades(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		resource resource.ResourceWithUpgradeState
		version  int64
		prior    string
		want     map[string]any
		gone     []string
	}{
		{&AppResource{}, 2, `{"id":"app-1","project_id":"p-1","name":"web"}`,
			map[string]any{"project": "p-1", "project_id": "p-1", "name": "web"}, nil},
		// The upgraders from before version 2 carry every later step too.
		{&AppResource{}, 1, `{"id":"app-1","project_id":"p-1","volume_mounts":null}`,
			map[string]any{"project": "p-1", "project_id": "p-1"}, nil},
		{&AppResource{}, 0, `{"id":"app-1","project_id":"p-1","mode":"always-on","replicas":1}`,
			map[string]any{"project": "p-1", "project_id": "p-1"}, []string{"mode", "replicas"}},
		{&DatabaseResource{}, 0, `{"id":"db-1","project_id":"p-1"}`,
			map[string]any{"project": "p-1", "project_id": "p-1"}, nil},
		{&SecretResource{}, 0, `{"id":"s-1","project_id":null,"organization_id":"o-1","scope":"organization"}`,
			map[string]any{"project": nil, "project_id": nil, "org": "o-1", "organization_id": "o-1"}, nil},
		{&ServiceAccountResource{}, 0, `{"id":"sa-1","project_id":"p-1","organization_id":null}`,
			map[string]any{"project": "p-1", "project_id": "p-1", "org": nil, "organization_id": nil}, nil},
		{&IAMBindingResource{}, 1, `{"id":"b-1","org_id":null,"project_id":"p-1","role":"viewer","member":"user:a@example.com"}`,
			map[string]any{"project": "p-1", "project_id": "p-1", "org": nil, "organization_id": nil}, []string{"org_id"}},
		{&RegistryRetentionPolicyResource{}, 0, `{"id":"rp-1","project_id":"p-1","repo":""}`,
			map[string]any{"project": "p-1", "project_id": "p-1"}, nil},
		{&RegistryVisibilityResource{}, 0, `{"id":"p-1/web","project_id":"p-1","repo":"web"}`,
			map[string]any{"project": "p-1", "project_id": "p-1", "id": "p-1/web"}, nil},
		{&BillingBudgetResource{}, 0, `{"org_id":"o-1","amount":"250.00"}`,
			map[string]any{"org": "o-1", "organization_id": "o-1", "amount": "250.00"}, []string{"org_id"}},
	} {
		var meta resource.MetadataResponse
		tc.resource.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "fpcloud"}, &meta)
		name := meta.TypeName
		upgrader, ok := tc.resource.UpgradeState(ctx)[tc.version]
		if !ok {
			t.Errorf("%s: no upgrader from version %d", name, tc.version)
			continue
		}
		var resp resource.UpgradeStateResponse
		upgrader.StateUpgrader(ctx, resource.UpgradeStateRequest{RawState: &tfprotov6.RawState{JSON: []byte(tc.prior)}}, &resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("%s from version %d: %v", name, tc.version, resp.Diagnostics)
			continue
		}
		var got map[string]any
		if err := json.Unmarshal(resp.DynamicValue.JSON, &got); err != nil {
			t.Fatal(err)
		}
		for k, v := range tc.want {
			if got[k] != v {
				t.Errorf("%s from version %d: %s is %v, want %v", name, tc.version, k, got[k], v)
			}
		}
		for _, k := range tc.gone {
			if _, ok := got[k]; ok {
				t.Errorf("%s from version %d: %s should have left the state", name, tc.version, k)
			}
		}

		// What the framework does with the answer: read it as the current
		// schema's value, refusing an attribute the schema does not have.
		var sch resource.SchemaResponse
		tc.resource.Schema(ctx, resource.SchemaRequest{}, &sch)
		if sch.Schema.Version <= tc.version {
			t.Errorf("%s: schema version %d is not past the version %d upgraded from", name, sch.Schema.Version, tc.version)
		}
		if _, err := resp.DynamicValue.Unmarshal(sch.Schema.Type().TerraformType(ctx).(tftypes.Object)); err != nil {
			t.Errorf("%s from version %d: the upgraded state does not fit the schema: %v", name, tc.version, err)
		}
	}
}
