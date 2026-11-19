package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

func upgradeFirewallState(t *testing.T, prior string) (map[string]any, string) {
	t.Helper()
	upgrader, ok := (&FirewallResource{}).UpgradeState(context.Background())[0]
	if !ok {
		t.Fatal("no upgrader from version 0")
	}
	var resp resource.UpgradeStateResponse
	upgrader.StateUpgrader(context.Background(), resource.UpgradeStateRequest{
		RawState: &tfprotov6.RawState{JSON: []byte(prior)},
	}, &resp)
	if resp.Diagnostics.HasError() {
		return nil, resp.Diagnostics.Errors()[0].Detail()
	}
	var got map[string]any
	if err := json.Unmarshal(resp.DynamicValue.JSON, &got); err != nil {
		t.Fatal(err)
	}
	return got, ""
}

// Version 0 held the rules as a list with the priority inside each rule; the
// upgrade keys each rule by its priority, drops the priority from inside it,
// and touches nothing else.
func TestUpgradeFirewallStateV0KeysRulesByPriority(t *testing.T) {
	prior := `{"id":"fw-1","name":"office","revision":4,"findings":[],"rules":[` +
		`{"priority":200,"action":"allow","description":"","enabled":true,"match":{"sources":["10.0.0.5"]}},` +
		`{"priority":100,"action":"deny","description":"one address","enabled":false,"match":{"sources":["192.0.2.13/32"]}}]}`
	got, failed := upgradeFirewallState(t, prior)
	if failed != "" {
		t.Fatal(failed)
	}
	out, _ := json.Marshal(got["rules"])
	want := `{"100":{"action":"deny","description":"one address","enabled":false,"match":{"sources":["192.0.2.13/32"]}},` +
		`"200":{"action":"allow","description":"","enabled":true,"match":{"sources":["10.0.0.5"]}}}`
	if string(out) != want {
		t.Errorf("rules upgraded to\n%s\nwant\n%s", out, want)
	}
	if got["id"] != "fw-1" || got["revision"] != float64(4) {
		t.Errorf("other attributes must be untouched, got %v", got)
	}
}

// No rules is null or empty either way: the list `[]` a configuration wrote
// is the map `{}` it now writes.
func TestUpgradeFirewallStateV0KeepsNoRules(t *testing.T) {
	for prior, want := range map[string]string{
		`{"id":"fw-1","rules":null}`: `null`,
		`{"id":"fw-1","rules":[]}`:   `{}`,
		`{"id":"fw-1"}`:              `null`,
	} {
		got, failed := upgradeFirewallState(t, prior)
		if failed != "" {
			t.Fatalf("%s: %s", prior, failed)
		}
		if out, _ := json.Marshal(got["rules"]); string(out) != want {
			t.Errorf("%s: rules upgraded to %s, want %s", prior, out, want)
		}
	}
}

// A state no version 0 could have written is refused rather than guessed at.
func TestUpgradeFirewallStateV0RefusesWhatNoRuleHeld(t *testing.T) {
	for prior, want := range map[string]string{
		`{"rules":[{"priority":100,"action":"deny"},{"priority":100,"action":"allow"}]}`: "priority 100 is held by another rule",
		`{"rules":[{"priority":1.5,"action":"deny"}]}`:                                   "not a whole number",
		`{"rules":[{"action":"deny"}]}`:                                                  "rules[0].priority",
	} {
		_, failed := upgradeFirewallState(t, prior)
		if !strings.Contains(failed, want) {
			t.Errorf("%s: failed with %q, want it to say %q", prior, failed, want)
		}
	}
}

// The refusal of a key says what to write instead when the key is a priority
// in another spelling.
func TestParseRulePriority(t *testing.T) {
	for key, want := range map[string]int{"1": 1, "100": 100, "65535": 65535} {
		if got, err := parseRulePriority(key); err != nil || got != want {
			t.Errorf("parseRulePriority(%q) = %d, %v; want %d", key, got, err, want)
		}
	}
	for key, want := range map[string]string{
		"0100":   "write its key as 100",
		"+100":   "write its key as 100",
		" 100":   "write its key as 100",
		"0":      "out of range",
		"-100":   "out of range",
		"1.5":    "has no priority",
		"office": "has no priority",
		"65536":  "out of range",
	} {
		_, err := parseRulePriority(key)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("parseRulePriority(%q) = %v; want an error saying %q", key, err, want)
		}
	}
}
