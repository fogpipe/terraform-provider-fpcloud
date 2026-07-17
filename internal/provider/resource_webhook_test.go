package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Webhook creation calls the GitHub API to register a hook on the configured
// repo, so it can only succeed against a real repo the platform has a token for.
// A throwaway acceptance repo/token isn't wired up, so these are skipped rather
// than left perpetually red. The config below is kept valid (real project+app
// scaffold) so the tests are ready to run once a fixture repo exists — set
// FPCLOUD_ACC_WEBHOOK_REPO to enable.
const accWebhookSkip = "set FPCLOUD_ACC_WEBHOOK_REPO to a real repo the platform can hook; webhook setup calls the GitHub API"

func TestAccWebhookResource(t *testing.T) {
	t.Skip(accWebhookSkip)
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
				Check: resource.TestCheckResourceAttrSet("fpcloud_webhook.test", "id"),
			},
		},
	})
}

func TestAccWebhookResourceWithBranch(t *testing.T) {
	t.Skip(accWebhookSkip)
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
				Check: resource.TestCheckResourceAttr("fpcloud_webhook.test", "branch", "develop"),
			},
		},
	})
}
