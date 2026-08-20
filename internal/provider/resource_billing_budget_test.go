package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccBillingBudgetResource manages the acceptance org's one budget — an
// organization has at most one, so this test owns it for the duration of the
// run and deletes it on teardown. The throwaway org has no budget to protect.
func TestAccBillingBudgetResource(t *testing.T) {
	orgID := testAccOrgID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "fpcloud_billing_budget" "test" {
  org_id = %q
  amount = "250.00"
}
`, orgID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_billing_budget.test", "amount", "250.00"),
					resource.TestCheckResourceAttr("fpcloud_billing_budget.test", "currency", "EUR"),
				),
			},
			{
				// The budget is imported by the organization id alone — the org
				// has at most one, so the schema has no separate id attribute,
				// and org_id is what identifies the imported resource (#89).
				ResourceName:                         "fpcloud_billing_budget.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "org_id",
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_billing_budget.test"]
					return rs.Primary.Attributes["org_id"], nil
				},
			},
		},
	})
}
