package provider_test

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccWebhookResource(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "fpcloud_webhook" "test" {
						app_id        = "test-app"
						repo          = "fpcloud/my-app"
						image_pattern = "ghcr.io/fpcloud/my-app:{{sha}}"
					}
				`,
			},
		},
	})
}

func TestAccWebhookResourceWithBranch(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "fpcloud_webhook" "test" {
						app_id        = "test-app"
						repo          = "fpcloud/my-app"
						branch        = "develop"
						image_pattern = "ghcr.io/fpcloud/my-app:{{sha}}"
					}
				`,
			},
		},
	})
}
