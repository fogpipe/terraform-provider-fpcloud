package provider_test

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDomainResource(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "fpcloud_domain" "test" {
						app_id = "test-app"
						domain = "app.example.com"
					}
				`,
			},
		},
	})
}
