package provider_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccProjectResource_basic(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set, skipping acceptance test")
	}
	// Randomized so a transient delete failure can't leave a fixed-name project
	// that collides with the next run.
	name := acctest.RandomWithPrefix("tf-acc-proj")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %q
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_project.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_project.test", "name", name),
					resource.TestCheckResourceAttr("fpcloud_project.test", "egress", "restricted"),
					resource.TestCheckResourceAttrSet("fpcloud_project.test", "max_pods"),
					resource.TestCheckResourceAttrSet("fpcloud_project.test", "created_at"),
					resource.TestCheckResourceAttrSet("fpcloud_project.test", "updated_at"),
				),
			},
			// Update egress in place (no replacement)
			{
				Config: fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name   = %q
  egress = "https"
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_project.test", "egress", "https"),
				),
			},
			// Import
			{
				ResourceName:      "fpcloud_project.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestProjectResourceSchema(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set, skipping")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "fpcloud_project" "test" {
  name = "schema-validation-test"
}
`,
				PlanOnly: true,
			},
		},
	})
}
