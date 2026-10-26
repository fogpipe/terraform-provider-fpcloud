package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// A state written before the serverless runtime was deleted carries `mode` and
// `traffic`; the upgrade drops exactly those two and keeps every other
// attribute as it was.
func TestUpgradeAppStateV0DropsModeAndTraffic(t *testing.T) {
	prior := []byte(`{"id":"app-1","name":"web","mode":"always-on","traffic":[{"revision":"web-00001","percent":100,"url":""}],"replicas":2,"routes":null}`)
	var resp resource.UpgradeStateResponse
	upgradeAppStateV0(context.Background(), resource.UpgradeStateRequest{RawState: &tfprotov6.RawState{JSON: prior}}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	var got map[string]any
	if err := json.Unmarshal(resp.DynamicValue.JSON, &got); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"mode", "traffic"} {
		if _, ok := got[gone]; ok {
			t.Errorf("%q should have left the state", gone)
		}
	}
	if got["id"] != "app-1" || got["name"] != "web" {
		t.Errorf("other attributes must be untouched, got %v", got)
	}
	// `replicas` leaves too, as both bounds (resource_app_state_test.go).
	if _, ok := got["replicas"]; ok || got["min_scale"] != float64(2) || got["max_scale"] != float64(2) {
		t.Errorf("replicas should have become min_scale = max_scale = 2, got %v", got)
	}
	if _, ok := got["routes"]; !ok {
		t.Errorf("a null attribute is kept as null, not dropped")
	}
}
