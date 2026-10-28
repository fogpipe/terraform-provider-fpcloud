package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
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
