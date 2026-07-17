package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccBucketResource is a regression test for the "Provider produced
// inconsistent result after apply" bug that shipped when a bucket was created
// with no quota set (the static 0 default fought the server default). Step 1
// creates a bucket with NO quota and asserts the computed quotas settle to the
// server defaults; step 2 sets explicit quotas in place (no re-create); step 3
// round-trips through import.
func TestAccBucketResource(t *testing.T) {
	// Keep names short: the Garage global alias is "<project>-<bucket>" and must
	// satisfy S3's 63-char bucket-name limit, so long RandomWithPrefix names fail.
	projectName := acctest.RandomWithPrefix("tfa-p")
	bucketName := acctest.RandomWithPrefix("tfa-b")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBucketDestroy,
		Steps: []resource.TestStep{
			// Step 1: bucket with NO quota — must not error, quotas settle to
			// the server defaults (the exact case that broke).
			{
				Config: testAccBucketConfigNoQuota(projectName, bucketName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_bucket.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_bucket.test", "name", bucketName),
					resource.TestCheckResourceAttrSet("fpcloud_bucket.test", "quota_max_size"),
					resource.TestCheckResourceAttrSet("fpcloud_bucket.test", "quota_max_objects"),
					resource.TestCheckResourceAttrSet("fpcloud_bucket.test", "endpoint"),
					resource.TestCheckResourceAttrSet("fpcloud_bucket.test", "access_key_id"),
				),
			},
			// Step 2: set explicit quotas — mutable in place, no re-create.
			{
				Config: testAccBucketConfigWithQuota(projectName, bucketName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_bucket.test", "quota_max_size", "1073741824"),
					resource.TestCheckResourceAttr("fpcloud_bucket.test", "quota_max_objects", "1000"),
				),
			},
			// Step 3: import — secret_access_key is create-only and not
			// recoverable on import, so it is ignored.
			{
				ResourceName:            "fpcloud_bucket.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"secret_access_key"},
			},
		},
	})
}

func testAccBucketConfigNoQuota(projectName, bucketName string) string {
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

func testAccBucketConfigWithQuota(projectName, bucketName string) string {
	return fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %[1]q
}

resource "fpcloud_bucket" "test" {
  project           = fpcloud_project.test.id
  name              = %[2]q
  quota_max_size    = 1073741824
  quota_max_objects = 1000
}
`, projectName, bucketName)
}
