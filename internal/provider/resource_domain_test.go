package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccDomainResource attaches a custom domain to a real always-on public app
// (custom domains are a dedicated/always-on + ingress feature, ADR-030). The
// domain starts in pending_verification (a non-owned example.com subdomain needs
// TXT + pointing), which is the expected create outcome — the resource is
// created, not rejected.
func TestAccDomainResource(t *testing.T) {
	proj := accName("domp")
	app := accName("doma")
	domain := accName("dom") + ".example.com"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAppScaffold(proj, app) + fmt.Sprintf(`
resource "fpcloud_domain" "test" {
  app_id = fpcloud_app.scaffold.id
  domain = %q
}
`, domain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_domain.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_domain.test", "domain", domain),
					resource.TestCheckResourceAttrSet("fpcloud_domain.test", "status"),
				),
			},
			{
				// An existing domain is imported as "app_id/domain" — the API lists
				// domains per app and has no lookup by the domain's id
				// (fogpipe/cloud-workspace#32). Everything else is read back.
				ResourceName:      "fpcloud_domain.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_domain.test"]
					return rs.Primary.Attributes["app_id"] + "/" + rs.Primary.Attributes["domain"], nil
				},
			},
		},
	})
}
