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
  project_id  = fpcloud_project.test.id
  role        = "viewer"
  member_type = "serviceAccount"
  member_id   = fpcloud_service_account.member.email
}
`, proj),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_iam_binding.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_iam_binding.test", "role", "viewer"),
				),
			},
			{
				// An existing binding is imported as "project_id/id" — the API
				// lists bindings per project (#89). Everything is read back.
				ResourceName:      "fpcloud_iam_binding.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_iam_binding.test"]
					return rs.Primary.Attributes["project_id"] + "/" + rs.Primary.ID, nil
				},
			},
		},
	})
}
