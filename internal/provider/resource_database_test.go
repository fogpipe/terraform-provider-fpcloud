package provider_test

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDatabaseResource(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "fpcloud_database" "test" {
						project_id = "test-project"
						name       = "testdb"
					}
				`,
			},
		},
	})
}

func TestAccDatabaseResourceWithOptions(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "fpcloud_database" "test" {
						project_id = "test-project"
						name       = "testdb"
						engine     = "postgres"
						version    = "17"
						plan       = "standard"
					}
				`,
			},
		},
	})
}
