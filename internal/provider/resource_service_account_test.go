package provider_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/provider"
)

func TestAccServiceAccountResource(t *testing.T) {
	proj := accName("sap")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %q
}

resource "fpcloud_service_account" "test" {
  project_id = fpcloud_project.test.id
  name       = "deployer"
}
`, proj),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_service_account.test", "id"),
					resource.TestCheckResourceAttrSet("fpcloud_service_account.test", "email"),
				),
			},
			{
				// Imported as "project/<project_id>/<id>": the API lists
				// service accounts per owner and has no get-by-id, so the id
				// names which owner to look in (#89, #778). Everything is read
				// back, so nothing is ignored.
				ResourceName:      "fpcloud_service_account.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_service_account.test"]
					return "project/" + rs.Primary.Attributes["project_id"] + "/" + rs.Primary.ID, nil
				},
			},
		},
	})
}

// A machine identity belongs to a project or to the organization itself
// (fogpipe/cloud-workspace#778), and the provider has to be able to say which.
// This is the assertion that was missing: `organization_id` was baselined as a
// field Terraform did not need, on the reasoning that a service account belongs
// to the project the resource already names — true until the org-scoped kind
// shipped on the API, the CLI and the console, and false from that day. A
// baseline records a decision, and nothing was checking that the decision still
// held.
func TestServiceAccountOwnerIsProjectOrOrg(t *testing.T) {
	var resp fwresource.SchemaResponse
	provider.NewServiceAccountResource().Schema(context.Background(), fwresource.SchemaRequest{}, &resp)

	for _, name := range []string{"project_id", "organization_id"} {
		attr, ok := resp.Schema.Attributes[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("%s attribute missing or not a StringAttribute", name)
		}
		if attr.Required {
			t.Errorf("%s must not be Required: it is one of two owners, and requiring it makes the other unexpressible", name)
		}
		if !attr.Optional {
			t.Errorf("%s must be Optional so exactly one owner can be configured", name)
		}
		var replaces bool
		for _, pm := range attr.PlanModifiers {
			if strings.Contains(strings.ToLower(fmt.Sprintf("%T", pm)), "requiresreplace") {
				replaces = true
			}
		}
		if !replaces {
			t.Errorf("%s must force replacement: an account cannot change owner in place", name)
		}
	}
}

// An identity the organization holds directly, which no project owns and which
// outlives every project — the case a project-scoped resource cannot express at
// all.
func TestAccServiceAccountResource_org(t *testing.T) {
	orgID := testAccOrgID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "fpcloud_service_account" "org" {
  organization_id = %q
  name            = %q
}
`, orgID, accName("saorg")),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_service_account.org", "id"),
					resource.TestCheckResourceAttrSet("fpcloud_service_account.org", "email"),
					resource.TestCheckResourceAttr("fpcloud_service_account.org", "organization_id", orgID),
					// The owner it does not have is null, not empty: an
					// org-scoped account is in no project's listing.
					resource.TestCheckNoResourceAttr("fpcloud_service_account.org", "project_id"),
				),
			},
			{
				ResourceName:      "fpcloud_service_account.org",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_service_account.org"]
					return "org/" + rs.Primary.Attributes["organization_id"] + "/" + rs.Primary.ID, nil
				},
			},
		},
	})
}
