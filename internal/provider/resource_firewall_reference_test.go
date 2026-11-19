package provider_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/provider"
)

// `project` takes the project by name or id, the way the platform's own
// examples write it, and holds what was written: the API answers with the id,
// and writing that back would make every apply of a name inconsistent with its
// own plan. Whether the firewall moves is decided by project_id. Every applying
// step here fails on a plan that is not empty after it.
func TestFirewall_ProjectByName(t *testing.T) {
	api := startFakeFirewallAPI(t)
	config := func(project string) string {
		return fmt.Sprintf(`
resource "fpcloud_firewall" "office" {
  project = %q
  name    = "office"
  rules = {
    100 = { action = "deny", match = { sources = ["192.0.2.0/24"] } }
  }
}`, project)
	}
	name := "fpcloud_firewall.office"
	puts := func(want int) resource.TestCheckFunc {
		return func(*terraform.State) error {
			api.mu.Lock()
			defer api.mu.Unlock()
			if api.puts != want {
				return fmt.Errorf("the firewall's content was written %d time(s), want %d", api.puts, want)
			}
			return nil
		}
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("demo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "project", "demo"),
					resource.TestCheckResourceAttr(name, "project_id", "proj-1"),
					resource.TestCheckResourceAttr(name, "revision", "1"),
				),
			},
			{
				// An import by id has no spelling to keep, and takes the id.
				ResourceName:            name,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"project"},
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 || states[0].Attributes["project"] != "proj-1" || states[0].Attributes["project_id"] != "proj-1" {
						return fmt.Errorf("imported %v, want project and project_id proj-1", states)
					}
					return nil
				},
			},
			{
				// An import by name keeps the project as the import spelled it.
				ResourceName:      name,
				ImportState:       true,
				ImportStateId:     "demo/office",
				ImportStateVerify: true,
			},
			{
				// The same project by its id: an update in place that writes
				// nothing, and no new revision.
				Config: config("proj-1"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "project", "proj-1"),
					resource.TestCheckResourceAttr(name, "project_id", "proj-1"),
					resource.TestCheckResourceAttr(name, "revision", "1"),
					puts(0),
				),
			},
			{
				// A different project is a different firewall.
				Config: config("other"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "project", "other"),
					resource.TestCheckResourceAttr(name, "project_id", "proj-2"),
				),
			},
			{
				// A project that resolves to nothing fails the plan, before
				// anything is replaced.
				Config:      config("nowhere"),
				ExpectError: regexp.MustCompile(`Error resolving project`),
			},
		},
	})
}

// State written before project_id existed holds the API's id in `project` and
// no project_id. The first plan that names the project by name is an update in
// place, with the id it resolves to planned: a plan with -refresh=false meets
// exactly this state.
func TestFirewall_StateWithoutProjectIDTakesASpellingInPlace(t *testing.T) {
	ctx := context.Background()
	api := &fakeFirewallAPI{}
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	r := provider.NewFirewallResource()
	var configured fwresource.ConfigureResponse
	r.(fwresource.ResourceWithConfigure).Configure(ctx, fwresource.ConfigureRequest{ProviderData: client.New(srv.URL, "fp-test")}, &configured)
	modifier, ok := r.(fwresource.ResourceWithModifyPlan)
	if !ok {
		t.Fatal("fpcloud_firewall has no ModifyPlan, so nothing can tell another spelling of its project from another project")
	}

	sch := firewallSchema(t)
	typ := sch.Type().TerraformType(ctx).(tftypes.Object)
	value := func(project string, projectID any) tftypes.Value {
		attrs := map[string]tftypes.Value{}
		for name, at := range typ.AttributeTypes {
			attrs[name] = tftypes.NewValue(at, nil)
		}
		attrs["id"] = tftypes.NewValue(tftypes.String, "fw-1")
		attrs["name"] = tftypes.NewValue(tftypes.String, "office")
		attrs["project"] = tftypes.NewValue(tftypes.String, project)
		if _, ok := attrs["project_id"]; ok {
			attrs["project_id"] = tftypes.NewValue(tftypes.String, projectID)
		}
		return tftypes.NewValue(typ, attrs)
	}

	for _, tc := range []struct {
		project string
		replace bool
		planned string
	}{
		{"demo", false, "proj-1"},
		{"other", true, ""},
	} {
		req := fwresource.ModifyPlanRequest{
			State:  tfsdk.State{Schema: sch, Raw: value("proj-1", nil)},
			Config: tfsdk.Config{Schema: sch, Raw: value(tc.project, nil)},
			Plan:   tfsdk.Plan{Schema: sch, Raw: value(tc.project, nil)},
		}
		resp := fwresource.ModifyPlanResponse{Plan: req.Plan}
		modifier.ModifyPlan(ctx, req, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("%s: %v", tc.project, resp.Diagnostics)
		}
		if replace := len(resp.RequiresReplace) > 0; replace != tc.replace {
			t.Errorf("%s: replaces %v, want %v", tc.project, replace, tc.replace)
		}
		var planned types.String
		resp.Diagnostics.Append(resp.Plan.GetAttribute(ctx, path.Root("project_id"), &planned)...)
		if tc.planned != "" && planned.ValueString() != tc.planned {
			t.Errorf("%s: project_id planned as %s, want %s", tc.project, planned, tc.planned)
		}
	}
}

// `firewall` on an app takes the firewall by name or id, as the API's attach
// does. The wait for the edge keys on the id the API answered, never on the
// configured string, so an app attached by name returns as soon as the edge
// enforces the firewall instead of waiting out the timeout, and a rewrite onto
// the firewall's id attaches nothing.
func TestFirewall_AppAttachedByName(t *testing.T) {
	api := startFakeFirewallAPI(t)
	// A wait that never matches fails within the test rather than in five
	// minutes.
	provider.SetFirewallTimeout(t, 3*time.Second)

	firewall := `
resource "fpcloud_firewall" "office" {
  project = "proj-1"
  name    = "office"
}`
	app := "fpcloud_app.web"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: firewall + fmt.Sprintf(fakeFirewallApp, "firewall = fpcloud_firewall.office.name"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(app, "firewall", "office"),
					resource.TestCheckResourceAttr(app, "firewall_id", "fw-1"),
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
				Config: firewall + fmt.Sprintf(fakeFirewallApp, "firewall = fpcloud_firewall.office.id"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(app, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(app, "firewall", "fw-1"),
					resource.TestCheckResourceAttr(app, "firewall_id", "fw-1"),
					func(*terraform.State) error {
						api.mu.Lock()
						defer api.mu.Unlock()
						if api.attaches != 0 {
							return fmt.Errorf("another spelling of the attached firewall made %d attach(es)", api.attaches)
						}
						return nil
					},
				),
			},
			{
				// A firewall the project does not hold fails the plan.
				Config:      firewall + fmt.Sprintf(fakeFirewallApp, `firewall = "nowhere"`),
				ExpectError: regexp.MustCompile(`Error resolving firewall`),
			},
			{
				// Taken off before the destroy, which deletes the firewall.
				Config: firewall + fmt.Sprintf(fakeFirewallApp, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(app, "firewall"),
					resource.TestCheckNoResourceAttr(app, "firewall_id"),
				),
			},
		},
	})
}
