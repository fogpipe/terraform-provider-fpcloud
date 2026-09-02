package provider_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Webhook creation calls the GitHub API to register a hook on the configured
// repo, so it can only succeed against a real repo the platform has a token
// for, and no throwaway acceptance repo is wired up. Both tests below said so
// and skipped unconditionally, which meant setting FPCLOUD_ACC_WEBHOOK_REPO did
// nothing — the message named an input the test never read, so the suite
// reported webhooks as covered with no way to make them run
// (fogpipe/cloud-workspace#253). The variable is now the repo the config
// actually hooks, so naming one runs the test.
func accWebhookRepo(t *testing.T) string {
	t.Helper()
	repo := os.Getenv("FPCLOUD_ACC_WEBHOOK_REPO")
	if repo == "" {
		t.Skip("set FPCLOUD_ACC_WEBHOOK_REPO to a real repo the platform can hook; webhook setup calls the GitHub API")
	}
	return repo
}

func TestAccWebhookResource(t *testing.T) {
	repo := accWebhookRepo(t)
	proj := accName("whp")
	app := accName("wha")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAppScaffold(proj, app) + fmt.Sprintf(`
resource "fpcloud_webhook" "test" {
  app_id        = fpcloud_app.scaffold.id
  repo          = %[1]q
  image_pattern = "ghcr.io/%[1]s:{{sha}}"
}
`, repo),
				Check: resource.TestCheckResourceAttrSet("fpcloud_webhook.test", "id"),
			},
		},
	})
}

func TestAccWebhookResourceWithBranch(t *testing.T) {
	repo := accWebhookRepo(t)
	proj := accName("whbp")
	app := accName("whba")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAppScaffold(proj, app) + fmt.Sprintf(`
resource "fpcloud_webhook" "test" {
  app_id        = fpcloud_app.scaffold.id
  repo          = %[1]q
  branch        = "develop"
  image_pattern = "ghcr.io/%[1]s:{{sha}}"
}
`, repo),
				Check: resource.TestCheckResourceAttr("fpcloud_webhook.test", "branch", "develop"),
			},
		},
	})
}
