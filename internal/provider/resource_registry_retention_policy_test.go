package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccRegistryRetentionPolicyResource(t *testing.T) {
	proj := accName("rrp")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %q
}

resource "fpcloud_registry_retention_policy" "test" {
  project_id = fpcloud_project.test.id
  enabled    = true
  keep_last  = 10
}
`, proj),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_registry_retention_policy.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_registry_retention_policy.test", "repo", ""),
				),
			},
			{
				// The project-wide policy (repo "") is imported by the project
				// id alone; a per-repo policy appends "/repo" (#89). Everything
				// is read back.
				ResourceName:      "fpcloud_registry_retention_policy.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_registry_retention_policy.test"]
					return rs.Primary.Attributes["project_id"], nil
				},
			},
		},
	})
}
