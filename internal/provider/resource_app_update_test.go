package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccAppResource_imageBump is a regression test for the "Provider produced
// inconsistent result after apply" failures that broke every app update: an
// image bump came back as the resolved digest and the replica count came back as
// a rollout-transient 0, so a deploy that had in fact succeeded exited non-zero
// and every CI pipeline read it as a failed deploy. Step 1 creates the app,
// step 2 bumps only the image — the apply must succeed with the configured tag
// and replica count intact in state.
func TestAccAppResource_imageBump(t *testing.T) {
	projectName := acctest.RandomWithPrefix("tf-acc-bump-proj")
	appName := acctest.RandomWithPrefix("tf-acc-bump-app")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccAppConfigImage(projectName, appName, "nginx:1.27"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.test", "image", "nginx:1.27"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "replicas", "2"),
				),
			},
			{
				Config: testAccAppConfigImage(projectName, appName, "nginx:1.28"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// The tag, not the digest the platform pins the pod to.
					resource.TestCheckResourceAttr("fpcloud_app.test", "image", "nginx:1.28"),
					// A deploy must not disturb the replica count.
					resource.TestCheckResourceAttr("fpcloud_app.test", "replicas", "2"),
				),
			},
		},
	})
}

// testAccAppConfigImage renders an always-on app at a given image, kept minimal
// (public image, internal ingress) so it needs no DNS.
func testAccAppConfigImage(projectName, appName, image string) string {
	return fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %[1]q
}

resource "fpcloud_app" "test" {
  project_id = fpcloud_project.test.id
  name       = %[2]q
  image      = %[3]q
  ingress    = "internal"
  replicas   = 2
}
`, projectName, appName, image)
}
