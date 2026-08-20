package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBucketLifecycleRuleResource(t *testing.T) {
	proj := accName("blp")
	bucket := accName("blb")

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
  project = fpcloud_project.test.id
  name    = %q
}

resource "fpcloud_bucket_lifecycle_rule" "root" {
  bucket_id   = fpcloud_bucket.test.id
  expire_days = 30
}

resource "fpcloud_bucket_lifecycle_rule" "scoped" {
  bucket_id   = fpcloud_bucket.test.id
  prefix      = "logs/archive/"
  expire_days = 7
}
`, proj, bucket),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_bucket_lifecycle_rule.root", "id"),
					resource.TestCheckResourceAttr("fpcloud_bucket_lifecycle_rule.scoped", "prefix", "logs/archive/"),
				),
			},
			{
				// The whole-bucket rule (prefix "") is imported by the bucket id
				// alone (#89). Everything is read back.
				ResourceName:      "fpcloud_bucket_lifecycle_rule.root",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_bucket_lifecycle_rule.root"]
					return rs.Primary.Attributes["bucket_id"], nil
				},
			},
			{
				// A prefixed rule appends "/prefix" — and the prefix itself
				// contains slashes, so everything after the first "/" is it.
				ResourceName:      "fpcloud_bucket_lifecycle_rule.scoped",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_bucket_lifecycle_rule.scoped"]
					return rs.Primary.Attributes["bucket_id"] + "/" + rs.Primary.Attributes["prefix"], nil
				},
			},
		},
	})
}
