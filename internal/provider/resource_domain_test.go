package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccDomainResource attaches a custom domain to a real always-on public app
// (custom domains are a dedicated/always-on + ingress feature, ADR-030). The
// domain starts in pending_verification (a non-owned example.com subdomain needs
// TXT + pointing), which is the expected create outcome — the resource is
// created, not rejected.
func TestAccDomainResource(t *testing.T) {
	proj := acctest.RandomWithPrefix("tf-acc-dom-proj")
	app := acctest.RandomWithPrefix("tf-acc-dom-app")
	domain := acctest.RandomWithPrefix("tf-acc") + ".example.com"

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
		},
	})
}
