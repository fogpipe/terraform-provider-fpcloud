package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fogpipe/cloud-cli/pkg/client"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/provider"
)

// The API keeps one spelling of a range: the masked prefix. What the provider
// computes has to be that spelling exactly, since it decides whether two
// sources are one.
func TestCanonicalCIDR(t *testing.T) {
	for in, want := range map[string]string{
		"10.0.0.5":            "10.0.0.5/32",
		"10.0.0.5/32":         "10.0.0.5/32",
		"10.1.2.3/8":          "10.0.0.0/8",
		"0.0.0.0/0":           "0.0.0.0/0",
		"2001:db8::1":         "2001:db8::1/128",
		"2001:DB8:0:0::/32":   "2001:db8::/32",
		"2001:db8:ffff::1/32": "2001:db8::/32",
	} {
		got, err := provider.CanonicalCIDR(in)
		if err != nil || got != want {
			t.Errorf("CanonicalCIDR(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", " 10.0.0.5", "10.0.0.5/33", "example.com", "10.0.0", "fe80::1%eth0", "::ffff:10.0.0.5", "::ffff:10.0.0.0/104"} {
		if got, err := provider.CanonicalCIDR(in); err == nil {
			t.Errorf("CanonicalCIDR(%q) = %q; want it refused", in, got)
		}
	}
}

// A source the configuration wrote and the one the API answered with are the
// same value when they name the same range, and only then.
func TestCIDRSemanticEquality(t *testing.T) {
	equal := [][2]string{
		{"10.0.0.5", "10.0.0.5/32"},
		{"10.1.2.3/8", "10.0.0.0/8"},
		{"2001:DB8::/32", "2001:db8::/32"},
		{"2001:db8::1", "2001:db8:0:0:0:0:0:1/128"},
	}
	for _, pair := range equal {
		if !provider.CIDRSemanticallyEqual(pair[0], pair[1]) || !provider.CIDRSemanticallyEqual(pair[1], pair[0]) {
			t.Errorf("%q and %q are one range and were read as two", pair[0], pair[1])
		}
	}
	different := [][2]string{
		{"10.0.0.5", "10.0.0.6"},
		{"10.0.0.0/8", "10.0.0.0/16"},
		{"10.0.0.5", "not-an-address"},
		{"not-an-address", "not-an-address"},
	}
	for _, pair := range different {
		if provider.CIDRSemanticallyEqual(pair[0], pair[1]) {
			t.Errorf("%q and %q were read as one range", pair[0], pair[1])
		}
	}
}

func firewallSchema(t *testing.T) schema.Schema {
	t.Helper()
	var resp fwresource.SchemaResponse
	provider.NewFirewallResource().Schema(context.Background(), fwresource.SchemaRequest{}, &resp)
	return resp.Schema
}

// The shape the documentation promises: rules are a list written inline,
// identified by priority, with sources a set, and the two attributes that
// name the firewall replace it.
func TestFirewallSchema(t *testing.T) {
	sch := firewallSchema(t)
	if diags := sch.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatalf("schema is invalid: %v", diags)
	}

	for _, name := range []string{"project", "name"} {
		a := sch.Attributes[name].(schema.StringAttribute)
		if !a.Required || len(a.PlanModifiers) == 0 {
			t.Errorf("%s: want required and replacing, got required=%v with %d plan modifiers", name, a.Required, len(a.PlanModifiers))
		}
	}
	for _, name := range []string{"id", "revision", "findings"} {
		if a := sch.Attributes[name]; !a.IsComputed() || a.IsOptional() || a.IsRequired() {
			t.Errorf("%s: want computed only", name)
		}
	}

	rules, ok := sch.Attributes["rules"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("rules is %T, want a nested list", sch.Attributes["rules"])
	}
	if !rules.Optional || rules.Computed {
		t.Errorf("rules: want optional and not computed, so the configuration's list is the whole list")
	}
	nested := rules.NestedObject.Attributes
	for _, name := range []string{"priority", "action", "match"} {
		if !nested[name].IsRequired() {
			t.Errorf("rules.%s: want required", name)
		}
	}
	for _, name := range []string{"description", "enabled"} {
		if !nested[name].IsOptional() || !nested[name].IsComputed() {
			t.Errorf("rules.%s: want optional with a default", name)
		}
	}
	match := nested["match"].(schema.SingleNestedAttribute)
	sources, ok := match.Attributes["sources"].(schema.SetAttribute)
	if !ok {
		t.Fatalf("rules.match.sources is %T, want a set", match.Attributes["sources"])
	}
	if got := sources.ElementType.String(); got != "cidrType" {
		t.Errorf("rules.match.sources holds %s, want the type that reads one range as one value", got)
	}
}

// What the API refuses at apply is refused at plan, and a range listed twice
// under two spellings above all: the API would answer with one entry where the
// configuration wrote two.
func TestFirewallConfigIsValidatedAtPlan(t *testing.T) {
	ctx := context.Background()
	sch := firewallSchema(t)
	typ := sch.Type().TerraformType(ctx).(tftypes.Object)
	ruleType := typ.AttributeTypes["rules"].(tftypes.List).ElementType.(tftypes.Object)
	matchType := ruleType.AttributeTypes["match"].(tftypes.Object)

	type rule struct {
		priority int
		action   string
		sources  []string
	}
	validate := func(defaultAction string, rules ...rule) []string {
		elems := []tftypes.Value{}
		for _, r := range rules {
			sources := []tftypes.Value{}
			for _, s := range r.sources {
				sources = append(sources, tftypes.NewValue(tftypes.String, s))
			}
			elems = append(elems, tftypes.NewValue(ruleType, map[string]tftypes.Value{
				"priority":    tftypes.NewValue(tftypes.Number, r.priority),
				"action":      tftypes.NewValue(tftypes.String, r.action),
				"description": tftypes.NewValue(tftypes.String, nil),
				"enabled":     tftypes.NewValue(tftypes.Bool, nil),
				"match": tftypes.NewValue(matchType, map[string]tftypes.Value{
					"sources": tftypes.NewValue(matchType.AttributeTypes["sources"], sources),
				}),
			}))
		}
		attrs := map[string]tftypes.Value{}
		for name, at := range typ.AttributeTypes {
			attrs[name] = tftypes.NewValue(at, nil)
		}
		attrs["project"] = tftypes.NewValue(tftypes.String, "p")
		attrs["name"] = tftypes.NewValue(tftypes.String, "office")
		attrs["default_action"] = tftypes.NewValue(tftypes.String, defaultAction)
		attrs["rules"] = tftypes.NewValue(typ.AttributeTypes["rules"], elems)

		var resp fwresource.ValidateConfigResponse
		provider.NewFirewallResource().(fwresource.ResourceWithValidateConfig).ValidateConfig(ctx, fwresource.ValidateConfigRequest{
			Config: tfsdk.Config{Schema: sch, Raw: tftypes.NewValue(typ, attrs)},
		}, &resp)
		out := []string{}
		for _, d := range resp.Diagnostics.Errors() {
			out = append(out, d.Summary())
		}
		return out
	}

	cases := []struct {
		name          string
		defaultAction string
		rules         []rule
		want          []string
	}{
		{"an allowlist", "deny", []rule{{100, "allow", []string{"10.0.0.5", "2001:db8::/32"}}, {200, "allow", []string{"192.0.2.0/24"}}}, nil},
		{"a default action that is not one", "drop", nil, []string{"Invalid default action"}},
		{"an action that is not one", "allow", []rule{{100, "block", []string{"10.0.0.5"}}}, []string{"Invalid rule action"}},
		{"a priority out of range", "allow", []rule{{0, "deny", []string{"10.0.0.5"}}}, []string{"Invalid rule priority"}},
		{"a priority taken twice", "allow", []rule{{100, "deny", []string{"10.0.0.5"}}, {100, "deny", []string{"10.0.0.6"}}}, []string{"Duplicate rule priority"}},
		{"a source that is no range", "allow", []rule{{100, "deny", []string{"office.example.com"}}}, []string{"Invalid source"}},
		{"one range under two spellings", "allow", []rule{{100, "deny", []string{"10.0.0.5", "10.0.0.5/32"}}}, []string{"Duplicate source"}},
		{"a rule with no source", "allow", []rule{{100, "deny", nil}}, []string{"A rule names at least one source"}},
	}
	for _, tc := range cases {
		got := validate(tc.defaultAction, tc.rules...)
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// fakeFirewallAPI is the part of the API a firewall and its attachment to one
// app touch, holding what the real one holds: rules in priority order, sources
// masked and sorted, a revision that counts content, and an edge that enforces
// a change some reads after it is stored.
type fakeFirewallAPI struct {
	mu sync.Mutex

	fw   *client.Firewall
	puts int
	// conditional counts the updates that named a revision to be written on
	// condition of, which a configuration never does.
	conditional int
	// deniesCaller makes every write that changes what an attached firewall
	// enforces, and every attach, answer that it denies its caller.
	deniesCaller bool

	// edgeLag is how many reads a change stays pending for; edgeFails makes
	// the edge refuse the next change with this message.
	edgeLag   int
	edgeFails string

	appExists    bool
	attached     bool // the app asks for fw
	appliedRev   int  // what the edge enforces on the app
	enforced     bool // the edge enforces a firewall on the app at all
	pendingReads int
	failed       string
	pendingSeen  int
}

func (f *fakeFirewallAPI) canonical(in []client.FirewallRuleRequest) []client.FirewallRule {
	out := make([]client.FirewallRule, 0, len(in))
	for _, r := range in {
		sources := make([]string, 0, len(r.Match.Sources))
		for _, s := range r.Match.Sources {
			c, err := provider.CanonicalCIDR(s)
			if err != nil {
				panic(err)
			}
			sources = append(sources, c)
		}
		sort.Strings(sources)
		out = append(out, client.FirewallRule{
			Priority: r.Priority, Action: r.Action, Description: r.Description,
			Enabled: r.Enabled == nil || *r.Enabled, Match: client.FirewallMatch{Sources: sources},
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	return out
}

// changed starts the edge on a change to what the app should enforce.
func (f *fakeFirewallAPI) changed() {
	f.pendingReads = f.edgeLag
	f.failed = ""
	if f.edgeFails != "" {
		f.failed, f.edgeFails = f.edgeFails, ""
	}
}

// settle is one read of the edge: a pending change comes one read closer, and
// once none is left the edge enforces what was asked.
func (f *fakeFirewallAPI) settle() (status string) {
	if f.failed != "" {
		return "failed"
	}
	if f.pendingReads > 0 {
		f.pendingReads--
		f.pendingSeen++
		return "pending"
	}
	f.enforced = f.attached
	if f.attached {
		f.appliedRev = f.fw.Revision
	}
	return "applied"
}

func (f *fakeFirewallAPI) firewall() client.Firewall {
	out := *f.fw
	out.Targets = []client.FirewallTarget{}
	if f.attached {
		status := f.settle()
		out.Targets = append(out.Targets, client.FirewallTarget{
			Type: "app", ID: "app-1", Name: "web", AppliedRevision: f.appliedRev, Status: status, Message: f.failed,
		})
	}
	return out
}

func (f *fakeFirewallAPI) app() client.App {
	app := client.App{ID: "app-1", ProjectID: "proj-1", Name: "web", Image: "ghcr.io/acme/web:v1",
		Type: "web", Port: 8080, Ingress: "internal", Status: "running", MinScale: 1, MaxScale: 1,
		CPULimit: "500m", MemoryLimit: "512Mi",
		HealthCheckPath: "/", HealthCheckTimeout: 5, HealthCheckInterval: 10, HealthCheckRetries: 3}
	status := f.settle()
	switch {
	case f.attached:
		app.Firewall = &client.FirewallRef{ID: f.fw.ID, Name: f.fw.Name, Revision: f.fw.Revision,
			AppliedRevision: f.appliedRev, Status: status, Message: f.failed}
	case f.enforced || status != "applied":
		// A detach the edge has not carried out yet names no firewall.
		app.Firewall = &client.FirewallRef{Status: status, Message: f.failed}
	}
	return app
}

func (f *fakeFirewallAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	refuse := func(status int, code, message string) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
	}
	missing := func() { refuse(http.StatusNotFound, "NOT_FOUND", "not found") }
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/proj-1/firewalls":
		var req client.CreateFirewallRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.DefaultAction == "" {
			req.DefaultAction = "allow"
		}
		now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		f.fw = &client.Firewall{ID: "fw-1", ProjectID: "proj-1", Name: req.Name, Description: req.Description,
			Revision: 1, DefaultAction: req.DefaultAction, Rules: f.canonical(req.Rules), CreatedAt: now, UpdatedAt: now}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(f.firewall())
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/proj-1/firewalls":
		out := []client.Firewall{}
		if f.fw != nil {
			out = append(out, f.firewall())
		}
		_ = json.NewEncoder(w).Encode(out)
	case r.URL.Path == "/api/v1/firewalls/fw-1" && f.fw == nil:
		missing()
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/firewalls/fw-1":
		_ = json.NewEncoder(w).Encode(f.firewall())
	case r.Method == http.MethodPut && r.URL.Path == "/api/v1/firewalls/fw-1":
		var req client.UpdateFirewallRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.puts++
		if req.IfRevision != 0 {
			f.conditional++
		}
		rules := f.canonical(req.Rules)
		before, _ := json.Marshal([]any{f.fw.DefaultAction, f.fw.Rules})
		after, _ := json.Marshal([]any{req.DefaultAction, rules})
		f.fw.Description = req.Description
		answer := func() client.Firewall { return f.firewall() }
		if string(before) != string(after) {
			f.fw.DefaultAction, f.fw.Rules = req.DefaultAction, rules
			f.fw.Revision++
			if f.attached {
				f.changed()
				if f.deniesCaller {
					answer = func() client.Firewall {
						fw := f.firewall()
						fw.Findings = append(fw.Findings, fakeDeniesCaller("stored"))
						return fw
					}
				}
			}
		}
		_ = json.NewEncoder(w).Encode(answer())
	case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/firewalls/fw-1":
		if f.attached {
			refuse(http.StatusConflict, "FIREWALL_ATTACHED", "firewall office is attached to app web; detach it first")
			return
		}
		f.fw = nil
		w.WriteHeader(http.StatusNoContent)

	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/proj-1/apps":
		f.appExists = true
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(f.app())
	case strings.HasPrefix(r.URL.Path, "/api/v1/apps/app-1") && !f.appExists:
		missing()
	case r.URL.Path == "/api/v1/apps/app-1/deployments":
		_ = json.NewEncoder(w).Encode([]client.Deployment{{ID: "dep-1", AppID: "app-1", Image: "ghcr.io/acme/web:v1", Status: "active"}})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/app-1":
		_ = json.NewEncoder(w).Encode(f.app())
	case r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/app-1/firewall":
		var req client.AttachFirewallRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if f.fw == nil || req.Firewall != f.fw.ID {
			missing()
			return
		}
		f.attached = true
		f.changed()
		ref := client.FirewallRef{ID: f.fw.ID, Name: f.fw.Name, Revision: f.fw.Revision, Status: "pending"}
		if f.deniesCaller {
			ref.Findings = []client.FirewallFinding{fakeDeniesCaller("attached")}
		}
		_ = json.NewEncoder(w).Encode(ref)
	case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/apps/app-1/firewall":
		f.attached = false
		f.changed()
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/apps/app-1":
		f.appExists, f.attached, f.enforced = false, false, false
		w.WriteHeader(http.StatusNoContent)
	default:
		missing()
	}
}

func startFakeFirewallAPI(t *testing.T) *fakeFirewallAPI {
	t.Helper()
	terraformBinary(t)
	api := &fakeFirewallAPI{}
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	t.Setenv("FPCLOUD_API_URL", srv.URL)
	t.Setenv("FPCLOUD_API_KEY", "fp-test")
	provider.SetFirewallPoll(t, time.Millisecond)
	return api
}

const fakeFirewallApp = `
resource "fpcloud_app" "web" {
  project_id = "proj-1"
  name       = "web"
  image      = "ghcr.io/acme/web:v1"
  %s
}`

// The API answers with rules in priority order and every source as a masked
// CIDR, sorted. A configuration written in another order, with a bare address
// and an unmasked or upper-case range, has to apply to a state it plans no
// change against: every step here fails on a plan that is not empty after its
// apply. The last steps show the other side, that state holds the API's own
// spelling once nothing in the configuration says otherwise.
func TestFirewall_OrderAndSpellingPlanNoChange(t *testing.T) {
	api := startFakeFirewallAPI(t)

	written := `
resource "fpcloud_firewall" "office" {
  project        = "proj-1"
  name           = "office"
  default_action = "deny"
  rules = [
    {
      priority = 200
      action   = "allow"
      match    = { sources = ["2001:DB8::/32", "192.0.2.77/24", "10.0.0.5"] }
    },
    {
      priority    = 100
      action      = "deny"
      description = "one address inside the office range"
      match       = { sources = ["192.0.2.13"] }
    },
  ]
}`
	// The same two rules, the first one widened, with one added between them.
	edited := `
resource "fpcloud_firewall" "office" {
  project        = "proj-1"
  name           = "office"
  description    = "the office and the VPN"
  default_action = "deny"
  rules = [
    {
      priority = 200
      action   = "allow"
      match    = { sources = ["2001:DB8::/32", "192.0.2.77/24", "10.0.0.5", "198.51.100.9"] }
    },
    {
      priority = 150
      action   = "allow"
      enabled  = false
      match    = { sources = ["203.0.113.0/24"] }
    },
    {
      priority    = 100
      action      = "deny"
      description = "one address inside the office range"
      match       = { sources = ["192.0.2.13"] }
    },
  ]
}`
	canonical := `
resource "fpcloud_firewall" "office" {
  project        = "proj-1"
  name           = "office"
  default_action = "deny"
  rules = [
    {
      priority = 100
      action   = "allow"
      match    = { sources = ["10.0.0.5/32"] }
    },
  ]
}`
	empty := `
resource "fpcloud_firewall" "office" {
  project = "proj-1"
  name    = "office"
}`

	name := "fpcloud_firewall.office"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: written,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", "fw-1"),
					resource.TestCheckResourceAttr(name, "revision", "1"),
					resource.TestCheckResourceAttr(name, "description", ""),
					// The configuration's order and its spellings are what state holds.
					resource.TestCheckResourceAttr(name, "rules.#", "2"),
					resource.TestCheckResourceAttr(name, "rules.0.priority", "200"),
					resource.TestCheckResourceAttr(name, "rules.0.enabled", "true"),
					resource.TestCheckResourceAttr(name, "rules.0.description", ""),
					resource.TestCheckResourceAttr(name, "rules.0.match.sources.#", "3"),
					resource.TestCheckTypeSetElemAttr(name, "rules.0.match.sources.*", "10.0.0.5"),
					resource.TestCheckTypeSetElemAttr(name, "rules.0.match.sources.*", "192.0.2.77/24"),
					resource.TestCheckTypeSetElemAttr(name, "rules.0.match.sources.*", "2001:DB8::/32"),
					resource.TestCheckResourceAttr(name, "rules.1.priority", "100"),
					resource.TestCheckTypeSetElemAttr(name, "rules.1.match.sources.*", "192.0.2.13"),
					// The API holds the canonical form, in priority order.
					func(*terraform.State) error {
						api.mu.Lock()
						defer api.mu.Unlock()
						got, _ := json.Marshal(api.fw.Rules)
						want := `[{"priority":100,"action":"deny","description":"one address inside the office range","enabled":true,"match":{"sources":["192.0.2.13/32"]}},` +
							`{"priority":200,"action":"allow","description":"","enabled":true,"match":{"sources":["10.0.0.5/32","192.0.2.0/24","2001:db8::/32"]}}]`
						if string(got) != want {
							return fmt.Errorf("the API holds\n%s\nwant\n%s", got, want)
						}
						return nil
					},
				),
			},
			{
				Config: edited,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "revision", "2"),
					resource.TestCheckResourceAttr(name, "description", "the office and the VPN"),
					resource.TestCheckResourceAttr(name, "rules.#", "3"),
					resource.TestCheckResourceAttr(name, "rules.1.priority", "150"),
					resource.TestCheckResourceAttr(name, "rules.1.enabled", "false"),
					resource.TestCheckResourceAttr(name, "rules.0.match.sources.#", "4"),
					resource.TestCheckTypeSetElemAttr(name, "rules.0.match.sources.*", "10.0.0.5"),
					resource.TestCheckTypeSetElemAttr(name, "rules.0.match.sources.*", "198.51.100.9"),
				),
			},
			{
				Config: canonical,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "revision", "3"),
					resource.TestCheckResourceAttr(name, "rules.#", "1"),
				),
			},
			{
				// By id, then by project and name: a configuration written the
				// way the API spells it imports to exactly its own state.
				ResourceName:      name,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      name,
				ImportState:       true,
				ImportStateId:     "proj-1/office",
				ImportStateVerify: true,
			},
			{
				// Leaving the rules out is no rules: the list is the
				// configuration's, never whatever the API still held.
				Config: empty,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "default_action", "allow"),
					resource.TestCheckNoResourceAttr(name, "rules.#"),
					func(*terraform.State) error {
						api.mu.Lock()
						defer api.mu.Unlock()
						if len(api.fw.Rules) != 0 {
							return fmt.Errorf("the API still holds %d rules", len(api.fw.Rules))
						}
						return nil
					},
				),
			},
		},
	})
}

// A rule added outside the configuration is drift the next plan removes, and a
// rule the configuration holds keeps its place and its spelling through it.
func TestFirewall_RuleAddedOutsideIsRemoved(t *testing.T) {
	api := startFakeFirewallAPI(t)
	config := `
resource "fpcloud_firewall" "office" {
  project = "proj-1"
  name    = "office"
  rules = [
    { priority = 300, action = "deny", match = { sources = ["203.0.113.7"] } },
    { priority = 200, action = "deny", match = { sources = ["198.51.100.0/24"] } },
  ]
}`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig: func() {
					api.mu.Lock()
					defer api.mu.Unlock()
					api.fw.Rules = append([]client.FirewallRule{{Priority: 50, Action: "allow", Enabled: true,
						Match: client.FirewallMatch{Sources: []string{"0.0.0.0/0"}}}}, api.fw.Rules...)
					api.fw.Revision++
				},
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_firewall.office", "rules.#", "2"),
					resource.TestCheckResourceAttr("fpcloud_firewall.office", "rules.0.priority", "300"),
					resource.TestCheckTypeSetElemAttr("fpcloud_firewall.office", "rules.0.match.sources.*", "203.0.113.7"),
					func(*terraform.State) error {
						api.mu.Lock()
						defer api.mu.Unlock()
						if len(api.fw.Rules) != 2 || api.fw.Rules[0].Priority != 200 {
							return fmt.Errorf("the rule added outside is still there: %+v", api.fw.Rules)
						}
						return nil
					},
				),
			},
		},
	})
}

// Attaching, editing and detaching each return only once the edge enforces
// the result, and a firewall still attached is not deleted: the API's reason
// is the error.
func TestFirewall_AttachmentWaitsForTheEdge(t *testing.T) {
	api := startFakeFirewallAPI(t)
	api.edgeLag = 3

	// The machine running the apply is in none of the ranges below, as a CI
	// runner would be: every attach and every edit is answered with the
	// warning that it denies its caller, and is made all the same.
	api.deniesCaller = true

	firewall := func(source string) string {
		return fmt.Sprintf(`
resource "fpcloud_firewall" "office" {
  project        = "proj-1"
  name           = "office"
  default_action = "deny"
  rules = [
    { priority = 100, action = "allow", match = { sources = [%q] } },
  ]
}`, source)
	}
	attached := fmt.Sprintf(fakeFirewallApp, "firewall = fpcloud_firewall.office.id")
	// Not through a reference, so Terraform orders nothing between the two
	// and the firewall's own delete is what meets the attachment.
	attachedByLiteral := fmt.Sprintf(fakeFirewallApp, `firewall = "fw-1"`)
	detached := fmt.Sprintf(fakeFirewallApp, "")

	// seen is how many pending reads the step waited through.
	seen := func(want int, enforced bool, rev int) resource.TestCheckFunc {
		return func(*terraform.State) error {
			api.mu.Lock()
			defer api.mu.Unlock()
			if api.pendingSeen != want {
				return fmt.Errorf("the apply returned after %d pending reads, want %d", api.pendingSeen, want)
			}
			if api.enforced != enforced || (enforced && api.appliedRev != rev) {
				return fmt.Errorf("the edge enforces %v at revision %d, want %v at %d", api.enforced, api.appliedRev, enforced, rev)
			}
			api.pendingSeen = 0
			return nil
		}
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Created together: the app is attached in its own create.
				Config: firewall("192.0.2.0/24") + attached,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.web", "firewall", "fw-1"),
					seen(3, true, 1),
				),
			},
			{
				// An edit to an attached firewall waits for its target.
				Config: firewall("198.51.100.0/24") + attached,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_firewall.office", "revision", "2"),
					seen(3, true, 2),
					// The warning about the caller is the apply's, and is not
					// kept: the read that follows would not repeat it.
					resource.TestCheckResourceAttr("fpcloud_firewall.office", "findings.#", "0"),
					func(*terraform.State) error {
						api.mu.Lock()
						defer api.mu.Unlock()
						if api.puts == 0 || api.conditional != 0 {
							return fmt.Errorf("%d of %d updates were conditional on a revision; a configuration is written on none", api.conditional, api.puts)
						}
						return nil
					},
				),
			},
			{
				// The edge refuses the next revision: the apply fails with the
				// target's own message.
				PreConfig: func() {
					api.mu.Lock()
					defer api.mu.Unlock()
					api.edgeFails = "route web-host: the policy was not accepted"
				},
				Config:      firewall("203.0.113.0/24") + attached,
				ExpectError: regexp.MustCompile(`(?s)did not apply revision 3 of firewall office to app web:.*policy was not accepted`),
			},
			{
				// The firewall was stored all the same, and state says so:
				// nothing is left to change once the edge has taken it.
				PreConfig: func() {
					api.mu.Lock()
					defer api.mu.Unlock()
					api.failed, api.pendingReads, api.pendingSeen = "", 0, 0
				},
				Config:   firewall("203.0.113.0/24") + attached,
				PlanOnly: true,
			},
			{
				Config:      attachedByLiteral,
				ExpectError: regexp.MustCompile(`(?s)Firewall is still attached.*firewall office is attached to app web`),
			},
			{
				// Detaching waits until the edge has let go.
				Config: firewall("203.0.113.0/24") + detached,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("fpcloud_app.web", "firewall"),
					seen(3, false, 0),
				),
			},
			{
				// Attached to an app that already exists.
				Config: firewall("203.0.113.0/24") + attached,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.web", "firewall", "fw-1"),
					seen(3, true, 3),
				),
			},
			{
				// And taken off again, so the destroy that ends the test
				// meets a firewall attached to nothing.
				Config: firewall("203.0.113.0/24") + detached,
				Check:  seen(3, false, 0),
			},
		},
	})
}

func fakeDeniesCaller(state string) client.FirewallFinding {
	return client.FirewallFinding{Code: "denies_caller", Message: "192.0.2.50, the address you are connecting from, is denied by this firewall as it is now " + state + " (by its default action)."}
}
