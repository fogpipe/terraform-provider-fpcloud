package provider

import (
	"strings"
	"testing"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// A finding is a warning on the attribute that caused it. One about the
// firewall as a whole has no field to name, and the one about the caller is
// shown on the apply and kept out of state, where no read would repeat it.
func TestFirewallFindingsAreWarnings(t *testing.T) {
	fw := &client.Firewall{Name: "office", Findings: []client.FirewallFinding{
		{Code: "shadowed", Field: "rules[1]", Message: "rule 200 never matches"},
		{Code: findingDeniesCaller, Message: "192.0.2.50, the address this request came from, is denied by this firewall as it is now stored (by its default action)."},
	}}
	var diags diag.Diagnostics
	warnFirewallFindings(fw, &diags)
	if diags.HasError() || diags.WarningsCount() != 2 {
		t.Fatalf("want two warnings and no error, got %v", diags)
	}
	if got := diags[0].Summary(); got != "Firewall office: rules[1]" {
		t.Errorf("summary = %q", got)
	}
	if got := diags[1].Summary(); got != "Firewall office" {
		t.Errorf("a finding with no field is titled %q", got)
	}
	if !strings.Contains(diags[1].Detail(), "the address this request came from") {
		t.Errorf("detail = %q", diags[1].Detail())
	}

	var m FirewallResourceModel
	var set diag.Diagnostics
	setModelFromFirewall(&m, fw, &set)
	if set.HasError() {
		t.Fatal(set)
	}
	if got := len(m.Findings.Elements()); got != 1 {
		t.Errorf("state keeps %d findings, want the one about the firewall", got)
	}
}
