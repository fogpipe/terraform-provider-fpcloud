package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccAppResource_mode is a regression test for the app tier→mode rename that
// shipped a "Provider produced inconsistent result after apply" bug. Step 1
// creates an app with `mode` OMITTED and asserts it defaults to always-on with
// no inconsistency; step 2 flips it to serverless (a replace, RequiresReplace on
// mode). The app is kept minimal (public image, internal ingress) so it needs no
// DNS.
func TestAccAppResource_mode(t *testing.T) {
	projectName := acctest.RandomWithPrefix("tf-acc-mode-proj")
	appName := acctest.RandomWithPrefix("tf-acc-mode-app")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppDestroy,
		Steps: []resource.TestStep{
			// Step 1: mode omitted — must default to always-on, no inconsistency.
			{
				Config: testAccAppConfigMode(projectName, appName, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_app.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "name", appName),
					resource.TestCheckResourceAttr("fpcloud_app.test", "mode", "always-on"),
				),
			},
			// Step 2: explicit serverless (replaces the app — RequiresReplace).
			{
				Config: testAccAppConfigMode(projectName, appName, "serverless"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.test", "mode", "serverless"),
				),
			},
		},
	})
}

// testAccAppConfigMode renders an app config. An empty mode omits the attribute
// so the provider default applies.
func testAccAppConfigMode(projectName, appName, mode string) string {
	modeLine := ""
	if mode != "" {
		modeLine = fmt.Sprintf("  mode    = %q\n", mode)
	}
	return fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %[1]q
}

resource "fpcloud_app" "test" {
  project_id = fpcloud_project.test.id
  name       = %[2]q
  image      = "nginx:latest"
  ingress    = "internal"
%[3]s}
`, projectName, appName, modeLine)
}
