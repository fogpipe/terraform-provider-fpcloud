package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccOrgSecretResource(t *testing.T) {
	orgID := testAccOrgID(t)
	name := accName("sec")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "fpcloud_org_secret" "test" {
  org_id = %q
  name   = %q
  data = {
    API_KEY = "v1"
  }
}
`, orgID, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_org_secret.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_org_secret.test", "data.API_KEY", "v1"),
				),
			},
			{
				// An existing bundle is imported as "org_id/name" (#89). Import
				// is complete: Read reveals the data, so even the values are
				// compared rather than ignored.
				ResourceName:      "fpcloud_org_secret.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_org_secret.test"]
					return rs.Primary.Attributes["org_id"] + "/" + rs.Primary.Attributes["name"], nil
				},
			},
		},
	})
}
