package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccBucketDomainResource attaches a custom domain to a website-enabled
// bucket. Like the app domain test, pending_verification is the expected
// create outcome for a hostname nobody proves ownership of.
func TestAccBucketDomainResource(t *testing.T) {
	proj := accName("bdp")
	bucket := accName("bdb")
	domain := accName("bdd") + ".example.com"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %q
}

resource "fpcloud_bucket" "test" {
  project         = fpcloud_project.test.id
  name            = %q
  website_enabled = true
}

resource "fpcloud_bucket_domain" "test" {
  bucket_id = fpcloud_bucket.test.id
  domain    = %q
}
`, proj, bucket, domain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_bucket_domain.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_bucket_domain.test", "domain", domain),
				),
			},
			{
				// An existing bucket domain is imported as "bucket_id/domain" —
				// the API lists domains per bucket (#89). status and tls_status
				// are the server's own verification lifecycle and move between
				// reads (listing lazily reconciles), so they are not compared.
				ResourceName:            "fpcloud_bucket_domain.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"status", "tls_status"},
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_bucket_domain.test"]
					return rs.Primary.Attributes["bucket_id"] + "/" + rs.Primary.Attributes["domain"], nil
				},
			},
		},
	})
}
