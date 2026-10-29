package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// A state written against schema version 0 names its member as a type and an
// id; the upgrade joins them into one `member` and adds the scope attribute
// that did not exist, leaving everything else as it was.
func TestUpgradeIAMBindingStateV0JoinsTheMember(t *testing.T) {
	prior := []byte(`{"id":"b-1","project_id":"p-1","resource":"database/db-1","role":"database.user","member_type":"serviceAccount","member_id":"api@p.org.cloud.fogpipe.com","created_at":"2026-01-01T00:00:00Z"}`)
	var resp resource.UpgradeStateResponse
	upgradeIAMBindingStateV0(context.Background(), resource.UpgradeStateRequest{RawState: &tfprotov6.RawState{JSON: prior}}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	var got map[string]any
	if err := json.Unmarshal(resp.DynamicValue.JSON, &got); err != nil {
		t.Fatal(err)
	}
	if got["member"] != "serviceAccount:api@p.org.cloud.fogpipe.com" {
		t.Errorf("member should be the joined pair, got %v", got["member"])
	}
	for _, gone := range []string{"member_type", "member_id"} {
		if _, ok := got[gone]; ok {
			t.Errorf("%q should have left the state", gone)
		}
	}
	if v, ok := got["org_id"]; !ok || v != nil {
		t.Errorf("org_id should be present and null, got %v (present %v)", v, ok)
	}
	if got["id"] != "b-1" || got["project_id"] != "p-1" || got["resource"] != "database/db-1" || got["role"] != "database.user" {
		t.Errorf("other attributes must be untouched, got %v", got)
	}
}

// A member typed `user` by default in an old state is spelled so.
func TestUpgradeIAMBindingStateV0DefaultsTheTypeToUser(t *testing.T) {
	prior := []byte(`{"id":"b-2","project_id":"p-1","resource":null,"role":"viewer","member_type":"","member_id":"jane@example.com","created_at":"2026-01-01T00:00:00Z"}`)
	var resp resource.UpgradeStateResponse
	upgradeIAMBindingStateV0(context.Background(), resource.UpgradeStateRequest{RawState: &tfprotov6.RawState{JSON: prior}}, &resp)
	var got map[string]any
	if err := json.Unmarshal(resp.DynamicValue.JSON, &got); err != nil {
		t.Fatal(err)
	}
	if got["member"] != "user:jane@example.com" {
		t.Errorf("got %v", got["member"])
	}
}

// The member spelling the schema takes is the type and the address, joined.
func TestSplitMember(t *testing.T) {
	for _, c := range []struct {
		in, typ, id string
		ok          bool
	}{
		{"user:jane@example.com", "user", "jane@example.com", true},
		{"serviceAccount:api@p.org.cloud.fogpipe.com", "serviceAccount", "api@p.org.cloud.fogpipe.com", true},
		{"jane@example.com", "", "", false},
		{"group:ops", "", "", false},
		{"user:", "", "", false},
	} {
		typ, id, err := splitMember(c.in)
		if (err == nil) != c.ok || typ != c.typ || id != c.id {
			t.Errorf("splitMember(%q) = %q, %q, %v; want %q, %q, ok=%v", c.in, typ, id, err, c.typ, c.id, c.ok)
		}
	}
}

// A `moved` block from either resource this one replaced lands as the same
// binding on the organization, the member spelled with its type, so the next
// refresh reads the row the source held by its id.
func TestMoveStateIntoIAMBinding(t *testing.T) {
	ctx := context.Background()
	r := &IAMBindingResource{}
	var sch resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sch)

	for _, tc := range []struct {
		source, prior string
		want          IAMBindingResourceModel
	}{
		{
			source: "fpcloud_org_member",
			prior:  `{"id":"b-7","organization_id":"o-1","email":"ana@example.com","role":"editor","user_id":"","status":"invited"}`,
			want:   IAMBindingResourceModel{ID: types.StringValue("b-7"), OrgID: types.StringValue("o-1"), Role: types.StringValue("editor"), Member: types.StringValue("user:ana@example.com")},
		},
		{
			source: "fpcloud_billing_binding",
			prior:  `{"id":"b-8","org_id":"o-1","role":"billing.viewer","member_type":"user","member":"fin@example.com","created_at":"2026-01-01T00:00:00Z"}`,
			want:   IAMBindingResourceModel{ID: types.StringValue("b-8"), OrgID: types.StringValue("o-1"), Role: types.StringValue("billing.viewer"), Member: types.StringValue("user:fin@example.com")},
		},
	} {
		moved := false
		for _, m := range r.MoveState(ctx) {
			resp := resource.MoveStateResponse{TargetState: tfsdk.State{Schema: sch.Schema, Raw: tftypes.NewValue(sch.Schema.Type().TerraformType(ctx), nil)}}
			m.StateMover(ctx, resource.MoveStateRequest{SourceTypeName: tc.source, SourceRawState: &tfprotov6.RawState{JSON: []byte(tc.prior)}}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("%s: %v", tc.source, resp.Diagnostics)
			}
			if resp.TargetState.Raw.IsNull() {
				continue
			}
			moved = true
			var got IAMBindingResourceModel
			resp.Diagnostics.Append(resp.TargetState.Get(ctx, &got)...)
			if got.ID != tc.want.ID || got.OrgID != tc.want.OrgID || got.Role != tc.want.Role || got.Member != tc.want.Member || !got.ProjectID.IsNull() || !got.Resource.IsNull() {
				t.Errorf("%s moved as %+v", tc.source, got)
			}
		}
		if !moved {
			t.Errorf("no mover accepted %s", tc.source)
		}
	}
}
