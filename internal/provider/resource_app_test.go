package provider_test

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAppResource_basic(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set, skipping acceptance test")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: `
resource "fpcloud_project" "test" {
  name = "tf-acc-test-app-project"
}

resource "fpcloud_app" "test" {
  project_id   = fpcloud_project.test.id
  name         = "tf-acc-test-app"
  image        = "nginx:latest"
  port         = 80
  ingress      = "all"
  min_scale    = 1
  max_scale    = 5
  cpu_limit    = "250m"
  memory_limit = "256Mi"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_app.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "name", "tf-acc-test-app"),
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
				// Port is not returned by GetApp, so we skip verifying it on import.
				ImportStateVerifyIgnore: []string{"port"},
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
