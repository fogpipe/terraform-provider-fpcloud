package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccIAMBindingResource(t *testing.T) {
	proj := accName("iamp")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// The member is a service account in the same project: the API
				// canonicalizes the member and 404s one that does not exist, so
				// a synthetic email would be refused.
				Config: fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %q
}

resource "fpcloud_service_account" "member" {
  project_id = fpcloud_project.test.id
  name       = "bound"
}

resource "fpcloud_iam_binding" "test" {
  project_id = fpcloud_project.test.id
  role       = "viewer"
  member     = "serviceAccount:${fpcloud_service_account.member.email}"
}
`, proj),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_iam_binding.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_iam_binding.test", "role", "viewer"),
				),
			},
			{
				// An existing binding is imported as "project/<project_id>/<id>":
				// the API lists bindings per scope. Everything is read back.
				ResourceName:      "fpcloud_iam_binding.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_iam_binding.test"]
					return "project/" + rs.Primary.Attributes["project_id"] + "/" + rs.Primary.ID, nil
				},
			},
		},
	})
}

// A billing role is a binding on the organization like any other.
func TestAccIAMBindingResource_Organization(t *testing.T) {
	orgID := testAccOrgID(t)
	member := accName("bill") + "@example.com"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "fpcloud_iam_binding" "test" {
  org_id = %q
  role   = "billing.viewer"
  member = %q
}
`, orgID, "user:"+member),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_iam_binding.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_iam_binding.test", "member", "user:"+member),
				),
			},
			{
				ResourceName:      "fpcloud_iam_binding.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_iam_binding.test"]
					return "organization/" + rs.Primary.Attributes["org_id"] + "/" + rs.Primary.ID, nil
				},
			},
		},
	})
}
