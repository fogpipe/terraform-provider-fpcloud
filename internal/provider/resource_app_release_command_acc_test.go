package provider_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// An apply that adds a release command alongside a new image runs it on that
// deploy (fogpipe/cloud-workspace#214, ADR-110). Sent as a request of its own,
// the deploy that introduces a migration is the one deploy with no release
// phase — and it reports success, having rolled the new code out past the
// migration that was added to gate it.
func TestAccAppResource_theDeployThatAddsAReleaseCommandRunsIt(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set, skipping acceptance test")
	}
	projName := accName("apprc")
	appName := accName("rc")

	config := func(image, release string) string {
		return fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %[1]q
}

resource "fpcloud_app" "test" {
  project_id      = fpcloud_project.test.id
  name            = %[2]q
  image           = %[3]q
  type            = "worker"
  command         = ["sleep"]
  args            = ["infinity"]
  release_command = [%[4]s]

  # busybox declares no user, and the platform refuses a root image on deploy
  # (cloud-platform#737) — the same explicit opt-out the other busybox tests
  # here take, so this one stays about the release command.
  security_context = {
    run_as_user     = 1000
    run_as_non_root = false
  }
}
`, projName, appName, image, release)
	}

	var appID string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppDestroy,
		Steps: []resource.TestStep{
			{
				Config: config("busybox:1.37", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.test", "release_command.#", "0"),
					func(s *terraform.State) error {
						appID = s.RootModule().Resources["fpcloud_app.test"].Primary.ID
						return nil
					},
				),
			},
			{
				// The image and the release command move together, which is what
				// adding a migration looks like.
				Config: config("busybox:1.36", `"true"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.test", "release_command.0", "true"),
					func(s *terraform.State) error {
						deps, err := testAccClient().ListDeployments(context.Background(), appID)
						if err != nil {
							return err
						}
						if len(deps) == 0 {
							return fmt.Errorf("no deployments recorded for %s", appID)
						}
						latest := deps[0]
						if latest.Image != "busybox:1.36" {
							return fmt.Errorf("newest deployment is %q, wanted the one that added the release command", latest.Image)
						}
						if len(latest.ReleaseCommand) != 1 || latest.ReleaseCommand[0] != "true" {
							return fmt.Errorf("the deploy that added the release command ran %v", latest.ReleaseCommand)
						}
						return nil
					},
				),
			},
		},
	})
}
