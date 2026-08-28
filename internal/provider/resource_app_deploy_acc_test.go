package provider_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// An update that deploys a new image must leave the app's other attributes as
// the plan predicted them (cloud-platform#456). Two of them are produced by the
// server rather than echoed back verbatim, and both violated the framework's
// post-apply contract at once: the deploy wrote a replica count nobody sent, so
// an app scaled to 2 came back at 0, and the digest the platform resolves the
// tag to was returned as the app's image, so `busybox:1.37` came back as
// `busybox@sha256:…`. Either one fails the apply with "Provider produced
// inconsistent result after apply" — an error that reports a deploy that
// actually succeeded as a failed one, so CI gating on the exit code stops.
//
// Nothing in the provider's own test set changed `image` or `replicas`, which
// is why both went unnoticed here. Each step's apply is followed by the
// framework's own empty-plan check, so an attribute drifting on a deploy fails
// this test whether it drifts during the apply or after it.
func TestAccAppResource_deployKeepsWhatItWasNotAsked(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set, skipping acceptance test")
	}
	projName := accName("appdep")
	appName := accName("dep")

	config := func(image string, replicas int) string {
		return fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %[1]q
}

resource "fpcloud_app" "test" {
  project_id = fpcloud_project.test.id
  name       = %[2]q
  image      = %[3]q
  type       = "worker"
  mode       = "always-on"
  replicas   = %[4]d
  command    = ["sleep"]
  args       = ["infinity"]

  # Small enough that three replicas fit under the acceptance org's ceiling
  # (ADR-092) — the default 500m puts the third one over it.
  cpu_limit    = "100m"
  memory_limit = "128Mi"

  security_context = {
    run_as_user     = 1000
    run_as_non_root = false
  }
}
`, projName, appName, image, replicas)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppDestroy,
		Steps: []resource.TestStep{
			{
				Config: config("busybox:1.36", 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.test", "image", "busybox:1.36"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "replicas", "2"),
				),
			},
			{
				// The deploy: image changes, replicas is not mentioned. It has to
				// survive, and the image has to read back as the tag that was asked
				// for rather than the digest it resolves to.
				Config: config("busybox:1.37", 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.test", "image", "busybox:1.37"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "replicas", "2"),
				),
			},
			{
				// And the other direction: scaling with the image untouched.
				Config: config("busybox:1.37", 3),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.test", "image", "busybox:1.37"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "replicas", "3"),
				),
			},
			{
				// Both at once, which is what a version bump plus a capacity change
				// looks like in one apply.
				Config: config("busybox:1.36", 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.test", "image", "busybox:1.36"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "replicas", "2"),
				),
			},
		},
	})
}
