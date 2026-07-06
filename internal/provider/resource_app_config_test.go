package provider_test

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAppConfigResource(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "fpcloud_app_config" "test" {
						app_id = "test-app"
						key    = "DATABASE_URL"
						value  = "postgres://localhost/test"
					}
				`,
			},
		},
	})
}

func TestAccAppConfigResourceSecret(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "fpcloud_app_config" "secret" {
						app_id    = "test-app"
						key       = "API_SECRET"
						value     = "super-secret-value"
						is_secret = true
					}
				`,
			},
		},
	})
}
