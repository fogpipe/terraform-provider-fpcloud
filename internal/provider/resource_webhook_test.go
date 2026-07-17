package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccWebhookResource(t *testing.T) {
	proj := acctest.RandomWithPrefix("tf-acc-wh-proj")
	app := acctest.RandomWithPrefix("tf-acc-wh-app")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAppScaffold(proj, app) + `
resource "fpcloud_webhook" "test" {
  app_id        = fpcloud_app.scaffold.id
  repo          = "fpcloud/acc-test"
  image_pattern = "ghcr.io/fpcloud/acc-test:{{sha}}"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_webhook.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_webhook.test", "repo", "fpcloud/acc-test"),
				),
			},
		},
	})
}

func TestAccWebhookResourceWithBranch(t *testing.T) {
	proj := acctest.RandomWithPrefix("tf-acc-whb-proj")
	app := acctest.RandomWithPrefix("tf-acc-whb-app")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAppScaffold(proj, app) + `
resource "fpcloud_webhook" "test" {
  app_id        = fpcloud_app.scaffold.id
  repo          = "fpcloud/acc-test"
  branch        = "develop"
  image_pattern = "ghcr.io/fpcloud/acc-test:{{sha}}"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_webhook.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_webhook.test", "branch", "develop"),
				),
			},
		},
	})
}
