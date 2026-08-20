package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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
				// An existing service account is imported as "project_id/id" —
				// the API lists service accounts per project (#89). Everything
				// is read back, so nothing is ignored.
				ResourceName:      "fpcloud_service_account.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_service_account.test"]
					return rs.Primary.Attributes["project_id"] + "/" + rs.Primary.ID, nil
				},
			},
		},
	})
}
