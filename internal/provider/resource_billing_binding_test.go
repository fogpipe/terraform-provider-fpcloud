package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBillingBindingResource(t *testing.T) {
	orgID := testAccOrgID(t)
	// billing.viewer, never billing.admin: revoking the org's last admin is
	// refused by the server, and the test must not be able to end up there.
	member := accName("bill") + "@example.com"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "fpcloud_billing_binding" "test" {
  org_id = %q
  role   = "billing.viewer"
  member = %q
}
`, orgID, member),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_billing_binding.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_billing_binding.test", "member_type", "user"),
				),
			},
			{
				// An existing binding is imported as "org_id/member" — the pair
				// the server's grant upserts on (#89). Everything is read back.
				ResourceName:      "fpcloud_billing_binding.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_billing_binding.test"]
					return rs.Primary.Attributes["org_id"] + "/" + rs.Primary.Attributes["member"], nil
				},
			},
		},
	})
}
