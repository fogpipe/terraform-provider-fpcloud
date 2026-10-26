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
	if got["id"] != "app-1" || got["name"] != "web" || got["replicas"] != float64(2) {
		t.Errorf("other attributes must be untouched, got %v", got)
	}
	if _, ok := got["routes"]; !ok {
		t.Errorf("a null attribute is kept as null, not dropped")
	}
}
