package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
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
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
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

// The shape the documentation promises: rules are a map written inline, keyed
// by priority, with sources a set, the name replaces the firewall, and the
// project is a reference whose id decides whether it is replaced.
func TestFirewallSchema(t *testing.T) {
	sch := firewallSchema(t)
	if diags := sch.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatalf("schema is invalid: %v", diags)
	}
	if sch.Version != 1 {
		t.Errorf("schema version is %d; the map of rules is version 1, upgraded from the list of version 0", sch.Version)
	}

	if a := sch.Attributes["name"].(schema.StringAttribute); !a.Required || len(a.PlanModifiers) == 0 {
		t.Errorf("name: want required and replacing, got required=%v with %d plan modifiers", a.Required, len(a.PlanModifiers))
	}
	if a := sch.Attributes["project"].(schema.StringAttribute); !a.Required || len(a.PlanModifiers) != 0 {
		t.Errorf("project: want required and compared by project_id, got required=%v with %d plan modifiers", a.Required, len(a.PlanModifiers))
	}
	for _, name := range []string{"id", "project_id", "revision", "findings"} {
		if a := sch.Attributes[name]; a == nil || !a.IsComputed() || a.IsOptional() || a.IsRequired() {
			t.Errorf("%s: want computed only", name)
		}
	}

	rules, ok := sch.Attributes["rules"].(schema.MapNestedAttribute)
	if !ok {
		t.Fatalf("rules is %T, want a nested map keyed by priority", sch.Attributes["rules"])
	}
	if !rules.Optional || rules.Computed {
		t.Errorf("rules: want optional and not computed, so the configuration's rules are all the rules")
	}
	nested := rules.NestedObject.Attributes
	if _, ok := nested["priority"]; ok {
		t.Errorf("rules.priority: the priority is the key, and a second place to write it could disagree with it")
	}
	for _, name := range []string{"action", "match"} {
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
	ruleType := typ.AttributeTypes["rules"].(tftypes.Map).ElementType.(tftypes.Object)
	matchType := ruleType.AttributeTypes["match"].(tftypes.Object)

	type rule struct {
		key     string
		action  string
		sources []string
	}
	validate := func(defaultAction string, rules ...rule) []string {
		elems := map[string]tftypes.Value{}
		for _, r := range rules {
			sources := []tftypes.Value{}
			for _, s := range r.sources {
				sources = append(sources, tftypes.NewValue(tftypes.String, s))
			}
			elems[r.key] = tftypes.NewValue(ruleType, map[string]tftypes.Value{
				"action":      tftypes.NewValue(tftypes.String, r.action),
				"description": tftypes.NewValue(tftypes.String, nil),
				"enabled":     tftypes.NewValue(tftypes.Bool, nil),
				"match": tftypes.NewValue(matchType, map[string]tftypes.Value{
					"sources": tftypes.NewValue(matchType.AttributeTypes["sources"], sources),
				}),
			})
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
		{"an allowlist", "deny", []rule{{"100", "allow", []string{"10.0.0.5", "2001:db8::/32"}}, {"200", "allow", []string{"192.0.2.0/24"}}}, nil},
		{"the lowest and the highest priority", "allow", []rule{{"1", "deny", []string{"10.0.0.5"}}, {"65535", "deny", []string{"10.0.0.6"}}}, nil},
		{"a default action that is not one", "drop", nil, []string{"Invalid default action"}},
		{"an action that is not one", "allow", []rule{{"100", "block", []string{"10.0.0.5"}}}, []string{"Invalid rule action"}},
		{"a source that is no range", "allow", []rule{{"100", "deny", []string{"office.example.com"}}}, []string{"Invalid source"}},
		{"one range under two spellings", "allow", []rule{{"100", "deny", []string{"10.0.0.5", "10.0.0.5/32"}}}, []string{"Duplicate source"}},
		{"a rule with no source", "allow", []rule{{"100", "deny", nil}}, []string{"A rule names at least one source"}},
	}
	// A key is a priority in its one spelling: anything else is refused,
	// whether or not it could be read as a number, since two spellings of one
	// priority would be two keys for one rule.
	for _, key := range []string{"0100", "+100", "-100", " 100", "100 ", "1e2", "100.0", "1.5", "office", "", "0", "65536", "99999999999999999999"} {
		cases = append(cases, struct {
			name          string
			defaultAction string
			rules         []rule
			want          []string
		}{fmt.Sprintf("the key %q", key), "allow", []rule{{key, "deny", []string{"10.0.0.5"}}}, []string{"Invalid rule priority"}})
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
	// condition of, and ifRevision is the revision the last one named.
	conditional, ifRevision int
	// changedBeforePut is an edit someone else makes after the plan and before
	// its apply's write arrives: the next update finds it already stored.
	changedBeforePut func(fw *client.Firewall)
	// deniesCaller makes every write that changes what an attached firewall
	// enforces, and every attach, answer that it denies its caller.
	deniesCaller bool

	// edgeLag is how many reads a change stays pending for; edgeFails makes
	// the edge refuse the next change with this message.
	edgeLag   int
	edgeFails string

	appExists bool
	attached  bool // the app asks for fw
	// createdWithFirewall counts app creates that carried a firewall, and
	// attaches the attach requests made after a create.
	createdWithFirewall, attaches int
	appliedRev                    int  // what the edge enforces on the app
	enforced                      bool // the edge enforces a firewall on the app at all
	pendingReads                  int
	failed                        string
	pendingSeen                   int
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
	// The API resolves a project by name or id at the route boundary, and
	// whatever follows reads its id.
	project := ""
	if rest, ok := strings.CutPrefix(r.URL.Path, "/api/v1/projects/"); ok {
		ref, tail, _ := strings.Cut(rest, "/")
		p, known := fakeProjects[ref]
		if !known {
			missing()
			return
		}
		project = p.ID
		if tail == "" {
			_ = json.NewEncoder(w).Encode(p)
			return
		}
		r.URL.Path = "/api/v1/projects/" + p.ID + "/" + tail
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/"+project+"/firewalls":
		var req client.CreateFirewallRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.DefaultAction == "" {
			req.DefaultAction = "allow"
		}
		now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		f.fw = &client.Firewall{ID: "fw-1", ProjectID: project, Name: req.Name, Description: req.Description,
			Revision: 1, DefaultAction: req.DefaultAction, Rules: f.canonical(req.Rules), CreatedAt: now, UpdatedAt: now}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(f.firewall())
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/"+project+"/firewalls":
		out := []client.Firewall{}
		if f.fw != nil && f.fw.ProjectID == project {
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
		if f.changedBeforePut != nil {
			f.changedBeforePut(f.fw)
			f.fw.Revision++
			f.changedBeforePut = nil
		}
		if req.IfRevision != 0 {
			f.conditional++
			f.ifRevision = req.IfRevision
			if req.IfRevision != f.fw.Revision {
				refuse(http.StatusConflict, "FIREWALL_REVISION_CONFLICT", fmt.Sprintf(
					"firewall %q has changed since it was read: it is at revision %d, and this change was made to revision %d. "+
						"Nothing was stored. Look at the firewall as it is now and make the change again",
					f.fw.Name, f.fw.Revision, req.IfRevision))
				return
			}
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
		var req client.CreateAppRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.appExists = true
		created := f.app()
		// A firewall named on the create is attached by it, and the create's
		// answer is where its warning about the caller is.
		if req.Firewall != "" {
			if !f.names(req.Firewall) {
				f.appExists = false
				missing()
				return
			}
			f.attached = true
			f.createdWithFirewall++
			f.changed()
			created = f.app()
			if f.deniesCaller && created.Firewall != nil {
				created.Firewall.Findings = []client.FirewallFinding{fakeDeniesCaller("attached")}
			}
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(created)
	case strings.HasPrefix(r.URL.Path, "/api/v1/apps/app-1") && !f.appExists:
		missing()
	case r.URL.Path == "/api/v1/apps/app-1/deployments":
		_ = json.NewEncoder(w).Encode([]client.Deployment{{ID: "dep-1", AppID: "app-1", Image: "ghcr.io/acme/web:v1", Status: "active"}})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/app-1":
		_ = json.NewEncoder(w).Encode(f.app())
	case r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/app-1/firewall":
		var req client.AttachFirewallRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if !f.names(req.Firewall) {
			missing()
			return
		}
		f.attached = true
		f.attaches++
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

// fakeProjects is every project the fake API holds, by each spelling it
// resolves.
var fakeProjects = map[string]client.Project{
	"demo":   {ID: "proj-1", Name: "demo"},
	"proj-1": {ID: "proj-1", Name: "demo"},
	"other":  {ID: "proj-2", Name: "other"},
	"proj-2": {ID: "proj-2", Name: "other"},
}

// names reports whether an attach's reference names the firewall, by id or
// by name, as the API resolves it in the app's project.
func (f *fakeFirewallAPI) names(ref string) bool {
	return f.fw != nil && f.fw.ProjectID == "proj-1" && (ref == f.fw.ID || ref == f.fw.Name)
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
  project    = "proj-1"
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
  rules = {
    200 = {
      action = "allow"
      match  = { sources = ["2001:DB8::/32", "192.0.2.77/24", "10.0.0.5"] }
    }
    100 = {
      action      = "deny"
      description = "one address inside the office range"
      match       = { sources = ["192.0.2.13"] }
    }
  }
}`
	// The same two rules, the first one widened, with one added between them.
	edited := `
resource "fpcloud_firewall" "office" {
  project        = "proj-1"
  name           = "office"
  description    = "the office and the VPN"
  default_action = "deny"
  rules = {
    200 = {
      action = "allow"
      match  = { sources = ["2001:DB8::/32", "192.0.2.77/24", "10.0.0.5", "198.51.100.9"] }
    }
    150 = {
      action  = "allow"
      enabled = false
      match   = { sources = ["203.0.113.0/24"] }
    }
    100 = {
      action      = "deny"
      description = "one address inside the office range"
      match       = { sources = ["192.0.2.13"] }
    }
  }
}`
	canonical := `
resource "fpcloud_firewall" "office" {
  project        = "proj-1"
  name           = "office"
  default_action = "deny"
  rules = {
    100 = {
      action = "allow"
      match  = { sources = ["10.0.0.5/32"] }
    }
  }
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
					// The configuration's spellings are what state holds.
					resource.TestCheckResourceAttr(name, "rules.%", "2"),
					resource.TestCheckResourceAttr(name, "rules.200.enabled", "true"),
					resource.TestCheckResourceAttr(name, "rules.200.description", ""),
					resource.TestCheckResourceAttr(name, "rules.200.match.sources.#", "3"),
					resource.TestCheckTypeSetElemAttr(name, "rules.200.match.sources.*", "10.0.0.5"),
					resource.TestCheckTypeSetElemAttr(name, "rules.200.match.sources.*", "192.0.2.77/24"),
					resource.TestCheckTypeSetElemAttr(name, "rules.200.match.sources.*", "2001:DB8::/32"),
					resource.TestCheckResourceAttr(name, "rules.100.action", "deny"),
					resource.TestCheckTypeSetElemAttr(name, "rules.100.match.sources.*", "192.0.2.13"),
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
					resource.TestCheckResourceAttr(name, "rules.%", "3"),
					resource.TestCheckResourceAttr(name, "rules.150.enabled", "false"),
					resource.TestCheckResourceAttr(name, "rules.200.match.sources.#", "4"),
					resource.TestCheckTypeSetElemAttr(name, "rules.200.match.sources.*", "10.0.0.5"),
					resource.TestCheckTypeSetElemAttr(name, "rules.200.match.sources.*", "198.51.100.9"),
				),
			},
			{
				Config: canonical,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "revision", "3"),
					resource.TestCheckResourceAttr(name, "rules.%", "1"),
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
				// Leaving the rules out is no rules: the rules are the
				// configuration's, never whatever the API still held.
				Config: empty,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "default_action", "allow"),
					resource.TestCheckNoResourceAttr(name, "rules.%"),
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

// ruleChanges is what a plan does to the rules of one resource, read from
// before and after the way the plan renderer pairs them: a map by key and a
// list by position. It is written for both shapes so that the same assertion
// can be run against the list `rules` once was, which is the positive control
// for the map.
type ruleChanges struct{ added, removed, changed []string }

func planRuleChanges(before, after any) ruleChanges {
	elems := func(v any) map[string]any {
		out := map[string]any{}
		switch rules := v.(type) {
		case map[string]any:
			for k, r := range rules {
				out[k] = r
			}
		case []any:
			for i, r := range rules {
				out[fmt.Sprint(i)] = r
			}
		}
		return out
	}
	b, a := elems(before), elems(after)
	var c ruleChanges
	for k, r := range a {
		prior, ok := b[k]
		switch {
		case !ok:
			c.added = append(c.added, k)
		case !reflect.DeepEqual(prior, r):
			c.changed = append(c.changed, k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			c.removed = append(c.removed, k)
		}
	}
	sort.Strings(c.added)
	sort.Strings(c.removed)
	sort.Strings(c.changed)
	return c
}

// expectRuleChanges asserts that the plan updates the resource in place and
// that its rules change in exactly the keys named and no other.
type expectRuleChanges struct {
	resource string
	want     ruleChanges
}

func (e expectRuleChanges) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	for _, rc := range req.Plan.ResourceChanges {
		if rc.Address != e.resource {
			continue
		}
		if !rc.Change.Actions.Update() {
			resp.Error = fmt.Errorf("%s: planned %v, want an update in place", e.resource, rc.Change.Actions)
			return
		}
		before, _ := rc.Change.Before.(map[string]any)
		after, _ := rc.Change.After.(map[string]any)
		got := planRuleChanges(before["rules"], after["rules"])
		if fmt.Sprint(got) != fmt.Sprint(e.want) {
			resp.Error = fmt.Errorf("%s: the plan changes rules as added %v, removed %v, changed %v; want added %v, removed %v, changed %v",
				e.resource, got.added, got.removed, got.changed, e.want.added, e.want.removed, e.want.changed)
		}
		return
	}
	resp.Error = fmt.Errorf("%s is not in the plan", e.resource)
}

// putsSince counts the whole-firewall writes an apply made: one, whatever
// the plan changed in the rules, so every apply is one revision and no rule
// is ever absent between two of them.
func putsSince(api *fakeFirewallAPI, before *int, want int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		api.mu.Lock()
		defer api.mu.Unlock()
		if got := api.puts - *before; got != want {
			return fmt.Errorf("the apply made %d update(s), want %d", got, want)
		}
		*before = api.puts
		return nil
	}
}

// A rule is identified by its priority, and so is its place in a plan:
// inserting one between two others plans that rule and nothing else, editing
// one plans that rule in place, and neither the order the configuration is
// written in nor an import touches anything. Each apply is one write.
func TestFirewall_RulesArePlannedByPriority(t *testing.T) {
	api := startFakeFirewallAPI(t)
	name := "fpcloud_firewall.office"

	rule := map[string]string{
		"100": `{ action = "allow", description = "office", match = { sources = ["198.51.100.0/24"] } }`,
		"150": `{ action = "allow", description = "partner", match = { sources = ["192.0.2.0/24"] } }`,
		"200": `{ action = "deny", description = "abuser", match = { sources = ["203.0.113.0/24"] } }`,
		"300": `{ action = "allow", description = "vpn", match = { sources = ["203.0.113.7/32"] } }`,
		// 200 with a source added.
		"200+": `{ action = "deny", description = "abuser", match = { sources = ["203.0.113.0/24", "198.18.0.0/15"] } }`,
	}
	// config writes the rules named, in the order named, each under its
	// priority: "200+" is written as 200.
	config := func(rules ...string) string {
		var b strings.Builder
		for _, r := range rules {
			fmt.Fprintf(&b, "    %s = %s\n", strings.TrimSuffix(r, "+"), rule[r])
		}
		return fmt.Sprintf(`
resource "fpcloud_firewall" "office" {
  project        = "proj-1"
  name           = "office"
  default_action = "deny"
  rules = {
%s  }
}`, b.String())
	}

	puts := 0
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("100", "200", "300"),
				Check:  putsSince(api, &puts, 0),
			},
			{
				// 150 between 100 and 200: one key added, the rules after it
				// untouched, and one write with every rule in it.
				Config: config("100", "150", "200", "300"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{expectRuleChanges{name, ruleChanges{added: []string{"150"}}}},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					putsSince(api, &puts, 1),
					resource.TestCheckResourceAttr(name, "revision", "2"),
					resource.TestCheckResourceAttr(name, "rules.%", "4"),
					resource.TestCheckResourceAttr(name, "rules.150.description", "partner"),
					func(*terraform.State) error {
						api.mu.Lock()
						defer api.mu.Unlock()
						var got []int
						for _, r := range api.fw.Rules {
							got = append(got, r.Priority)
						}
						if fmt.Sprint(got) != "[100 150 200 300]" {
							return fmt.Errorf("the API holds rules %v", got)
						}
						// Written on condition of the revision the plan read.
						if api.ifRevision != 1 {
							return fmt.Errorf("the update was made to revision %d, want 1", api.ifRevision)
						}
						return nil
					},
				),
			},
			{
				// A source added to 200 is a change to 200 alone.
				Config: config("100", "150", "200+", "300"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{expectRuleChanges{name, ruleChanges{changed: []string{"200"}}}},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					putsSince(api, &puts, 1),
					resource.TestCheckResourceAttr(name, "rules.200.match.sources.#", "2"),
				),
			},
			{
				// The same rules written in another order are the same rules.
				Config: config("300", "150", "100", "200+"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: putsSince(api, &puts, 0),
			},
			{
				// Imported through an import block against that configuration,
				// which is not in the order the API answers in: a no-op.
				ResourceName:    name,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				ImportStateId:   "proj-1/office",
				Config:          config("300", "150", "100", "200+"),
			},
		},
	})
}

// A change made elsewhere after the plan read the firewall is not written
// over: the apply fails saying so and stores nothing. The next plan shows the
// change, and applying that plan is what overwrites it.
func TestFirewall_ChangeAfterThePlanFailsTheApply(t *testing.T) {
	api := startFakeFirewallAPI(t)
	name := "fpcloud_firewall.office"
	config := func(sources string) string {
		return fmt.Sprintf(`
resource "fpcloud_firewall" "office" {
  project = "proj-1"
  name    = "office"
  rules = {
    100 = { action = "deny", match = { sources = [%s] } }
  }
}`, sources)
	}
	outside := client.FirewallRule{Priority: 50, Action: "allow", Enabled: true,
		Match: client.FirewallMatch{Sources: []string{"0.0.0.0/0"}}}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config(`"192.0.2.0/24"`)},
			{
				PreConfig: func() {
					api.mu.Lock()
					defer api.mu.Unlock()
					api.changedBeforePut = func(fw *client.Firewall) {
						fw.Rules = append([]client.FirewallRule{outside}, fw.Rules...)
					}
				},
				Config: config(`"192.0.2.0/24", "198.51.100.0/24"`),
				ExpectError: regexp.MustCompile(`Firewall changed since the plan(?s:.*)office was changed outside this\s+configuration` +
					`(?s:.*)at\s+revision\s+1(?s:.*)Nothing\s+was\s+written(?s:.*)Run\s+a\s+new\s+plan` +
					`(?s:.*)at\s+revision\s+2,\s+and\s+this\s+change\s+was\s+made\s+to\s+revision\s+1`),
			},
			{
				// Nothing of the refused apply was stored, and the change made
				// elsewhere is still there.
				PreConfig: func() {
					api.mu.Lock()
					defer api.mu.Unlock()
					if len(api.fw.Rules) != 2 || api.fw.Rules[0].Priority != 50 || len(api.fw.Rules[1].Match.Sources) != 1 {
						t.Errorf("after the refused apply the API holds %+v", api.fw.Rules)
					}
				},
				// The next plan shows that change being removed, and its apply,
				// made to the revision it read, is taken.
				Config: config(`"192.0.2.0/24", "198.51.100.0/24"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{expectRuleChanges{name, ruleChanges{removed: []string{"50"}, changed: []string{"100"}}}},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "revision", "3"),
					resource.TestCheckResourceAttr(name, "rules.%", "1"),
					func(*terraform.State) error {
						api.mu.Lock()
						defer api.mu.Unlock()
						if api.ifRevision != 2 || len(api.fw.Rules) != 1 {
							return fmt.Errorf("the update was made to revision %d and left %+v", api.ifRevision, api.fw.Rules)
						}
						return nil
					},
				),
			},
		},
	})
}

// A state written by the list `rules` of schema version 0 is upgraded to the
// map with nothing read from the API but the refresh, and the configuration
// rewritten as a map plans no change against it.
func TestFirewall_ListStateUpgradesToMap(t *testing.T) {
	api := startFakeFirewallAPI(t)
	dir := t.TempDir()
	config := `
resource "fpcloud_firewall" "office" {
  project        = "proj-1"
  name           = "office"
  default_action = "deny"
  rules = {
    200 = { action = "allow", match = { sources = ["10.0.0.5", "2001:DB8::/32"] } }
    100 = { action = "deny", description = "one address", enabled = false, match = { sources = ["192.0.2.13"] } }
  }
}`
	puts := 0
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		WorkingDir:               dir,
		Steps: []resource.TestStep{
			{Config: config},
			{
				// Write the state back as version 0 held it: a list, the
				// configuration's order, the priority inside each rule.
				PreConfig: func() { rewriteFirewallStateAsV0(t, dir) },
				Config:    config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					putsSince(api, &puts, 0),
					resource.TestCheckResourceAttr("fpcloud_firewall.office", "rules.100.enabled", "false"),
					resource.TestCheckTypeSetElemAttr("fpcloud_firewall.office", "rules.200.match.sources.*", "2001:DB8::/32"),
				),
			},
		},
	})
}

// rewriteFirewallStateAsV0 turns the firewall in the state file under dir
// back into what schema version 0 wrote.
func rewriteFirewallStateAsV0(t *testing.T, dir string) {
	t.Helper()
	var file string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Name() == "terraform.tfstate" {
			file = p
		}
		return nil
	})
	if file == "" {
		t.Fatalf("no state file under %s", dir)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	rewritten := false
	for _, res := range state["resources"].([]any) {
		res := res.(map[string]any)
		if res["type"] != "fpcloud_firewall" {
			continue
		}
		for _, inst := range res["instances"].([]any) {
			inst := inst.(map[string]any)
			attrs := inst["attributes"].(map[string]any)
			rules := attrs["rules"].(map[string]any)
			keys := make([]string, 0, len(rules))
			for k := range rules {
				keys = append(keys, k)
			}
			// Highest first, so the list is not in the order a map reads in.
			sort.Sort(sort.Reverse(sort.StringSlice(keys)))
			list := []any{}
			for _, k := range keys {
				rule := rules[k].(map[string]any)
				var priority int
				_, _ = fmt.Sscan(k, &priority)
				rule["priority"] = priority
				list = append(list, rule)
			}
			attrs["rules"] = list
			inst["schema_version"] = 0
			rewritten = true
		}
	}
	if !rewritten {
		t.Fatal("no firewall in the state file")
	}
	state["serial"] = state["serial"].(float64) + 1
	out, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, out, 0o600); err != nil {
		t.Fatal(err)
	}
}

// A rule added outside the configuration is drift the next plan removes, and a
// rule the configuration holds keeps its spelling through it.
func TestFirewall_RuleAddedOutsideIsRemoved(t *testing.T) {
	api := startFakeFirewallAPI(t)
	config := `
resource "fpcloud_firewall" "office" {
  project = "proj-1"
  name    = "office"
  rules = {
    300 = { action = "deny", match = { sources = ["203.0.113.7"] } }
    200 = { action = "deny", match = { sources = ["198.51.100.0/24"] } }
  }
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
					resource.TestCheckResourceAttr("fpcloud_firewall.office", "rules.%", "2"),
					resource.TestCheckTypeSetElemAttr("fpcloud_firewall.office", "rules.300.match.sources.*", "203.0.113.7"),
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
  rules = {
    100 = { action = "allow", match = { sources = [%q] } }
  }
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
				// Created together: the firewall rides on the app's create, so
				// the app is never served without it, and no attach follows.
				Config: firewall("192.0.2.0/24") + attached,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.web", "firewall", "fw-1"),
					seen(3, true, 1),
					func(*terraform.State) error {
						api.mu.Lock()
						defer api.mu.Unlock()
						if api.createdWithFirewall != 1 || api.attaches != 0 {
							return fmt.Errorf("the create carried the firewall %d time(s) and %d attach(es) followed; want 1 and 0", api.createdWithFirewall, api.attaches)
						}
						return nil
					},
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
						if api.puts == 0 || api.conditional != api.puts {
							return fmt.Errorf("%d of %d updates were conditional on a revision; every one is made to the revision its plan read", api.conditional, api.puts)
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
	return client.FirewallFinding{Code: "denies_caller", Message: "192.0.2.50, the address this request came from, is denied by this firewall as it is now " + state + " (by its default action)."}
}
