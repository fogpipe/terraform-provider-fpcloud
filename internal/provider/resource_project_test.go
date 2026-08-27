package provider_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/provider"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

func TestAccProjectResource_basic(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set, skipping acceptance test")
	}
	// Randomized so a transient delete failure can't leave a fixed-name project
	// that collides with the next run.
	name := accName("proj")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %q
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_project.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_project.test", "name", name),
					resource.TestCheckResourceAttr("fpcloud_project.test", "egress", "restricted"),
					resource.TestCheckResourceAttrSet("fpcloud_project.test", "created_at"),
					resource.TestCheckResourceAttrSet("fpcloud_project.test", "updated_at"),
				),
			},
			// Update egress in place (no replacement)
			{
				Config: fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name   = %q
  egress = "https"
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_project.test", "egress", "https"),
				),
			},
			// Import
			{
				ResourceName:      "fpcloud_project.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestProjectResourceSchema(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set, skipping")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "fpcloud_project" "test" {
  name = "schema-validation-test"
}
`,
				PlanOnly: true,
			},
		},
	})
}

// The organization a project is in is a fact about the project; the string a
// configuration reaches it by is not. `org` answers to a uuid, an opaque id and
// a readable name alike, so a plan that compares the string calls a rename a
// move and destroys the project, its apps, its databases and its buckets with
// it (#213). What replacement keys on is the frozen id.
func TestProjectOrgIsAReferenceNotTheOrganization(t *testing.T) {
	var resp fwresource.SchemaResponse
	provider.NewProjectResource().Schema(context.Background(), fwresource.SchemaRequest{}, &resp)

	org, ok := resp.Schema.Attributes["org"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("org attribute missing or not a StringAttribute")
	}
	for _, pm := range org.PlanModifiers {
		if strings.Contains(strings.ToLower(fmt.Sprintf("%T", pm)), "requiresreplace") {
			t.Errorf("org replaces the project on a re-spelling; only a different organization may")
		}
	}
	if !org.Optional || !org.Computed {
		t.Errorf("org must be Optional+Computed so a resolved reference can be recorded")
	}

	orgID, ok := resp.Schema.Attributes["organization_id"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("organization_id attribute missing or not a StringAttribute")
	}
	if !orgID.Computed {
		t.Errorf("organization_id is read from the API, never configured")
	}
}

// The same organization, spelled two ways, across an apply: the second plan is
// an update at most. A replacement here is the whole project.
func TestAccProjectResource_orgRespellingIsNotAMove(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set, skipping acceptance test")
	}
	org := testAccOrg(t)
	if org.ShortID == "" || org.ShortID == org.ID {
		t.Skip("organization has no second spelling to compare")
	}
	name := accName("proj")
	config := func(spelling string) string {
		return fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %q
  org  = %q
}
`, name, spelling)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(org.ID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_project.test", "organization_id", org.ID),
				),
			},
			{
				Config: config(org.ShortID),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fpcloud_project.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_project.test", "org", org.ShortID),
					resource.TestCheckResourceAttr("fpcloud_project.test", "organization_id", org.ID),
				),
			},
		},
	})
}
