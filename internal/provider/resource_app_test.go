package provider_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAppResource_basic(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set, skipping acceptance test")
	}
	// Randomized so a transient delete failure can't leave fixed-name resources
	// that collide with the next run.
	projName := acctest.RandomWithPrefix("tf-acc-app-proj")
	appName := acctest.RandomWithPrefix("tf-acc-app")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %[1]q
}

resource "fpcloud_app" "test" {
  project_id   = fpcloud_project.test.id
  name         = %[2]q
  image        = "nginx:latest"
  port         = 80
  ingress      = "all"
  min_scale    = 1
  max_scale    = 5
  cpu_limit    = "250m"
  memory_limit = "256Mi"
}
`, projName, appName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_app.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "name", appName),
					resource.TestCheckResourceAttr("fpcloud_app.test", "image", "nginx:latest"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "port", "80"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "min_scale", "1"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "max_scale", "5"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "cpu_limit", "250m"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "memory_limit", "256Mi"),
					resource.TestCheckResourceAttrSet("fpcloud_app.test", "url"),
					resource.TestCheckResourceAttrSet("fpcloud_app.test", "created_at"),
					resource.TestCheckResourceAttrSet("fpcloud_app.test", "updated_at"),
				),
			},
			// Import
			{
				ResourceName:      "fpcloud_app.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Port is not returned by GetApp; status/updated_at are volatile as
				// the app reconciles (deploying → running) between create and import.
				ImportStateVerifyIgnore: []string{"port", "status", "updated_at"},
			},
		},
	})
}

func TestAppResourceSchema(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set, skipping")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "fpcloud_app" "test" {
  project_id = "fake-project-id"
  name       = "schema-validation-test"
  image      = "nginx:latest"
}
`,
				PlanOnly: true,
			},
		},
	})
}
