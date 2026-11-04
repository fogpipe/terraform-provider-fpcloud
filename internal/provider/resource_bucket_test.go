package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccBucketResource creates a bucket and round-trips it through import. A
// bucket has no size: what it stores is the organization's to bound.
func TestAccBucketResource(t *testing.T) {
	// The Garage global alias is "<project>-<bucket>" and must satisfy S3's
	// 63-char bucket-name limit — the same budget accName exists to stay inside.
	projectName := accName("bktp")
	bucketName := accName("bkt")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketConfig(projectName, bucketName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_bucket.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_bucket.test", "name", bucketName),
					resource.TestCheckResourceAttrSet("fpcloud_bucket.test", "endpoint"),
				),
			},
			// The bucket carries no key, so every attribute
			// reads back from the API and nothing is ignored.
			{
				ResourceName:      "fpcloud_bucket.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccBucketConfig(projectName, bucketName string) string {
	return fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %[1]q
}

resource "fpcloud_bucket" "test" {
  project = fpcloud_project.test.id
  name    = %[2]q
}
`, projectName, bucketName)
}
