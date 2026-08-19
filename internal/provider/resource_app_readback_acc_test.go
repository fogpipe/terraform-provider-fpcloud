package provider_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/fogpipe/cloud-cli/pkg/client"
)

// The fields the API echoes and the provider reads back
// (fogpipe/cloud-workspace#12): command, args, release_command, volume_mounts
// and security_context. Each step's apply is followed by the framework's own
// plan, which must be empty — so every field is proven to round-trip without a
// permanent diff, per field, on create and on update. The import step proves
// the values come from the API, not from the plan. The last steps change two of
// them outside Terraform and expect the plan to notice.
//
// busybox runs as root, so run_as_non_root is set to false explicitly — the
// opt-out the API accepts for such an image — which also proves an explicit
// false survives the round-trip rather than collapsing into null.
func TestAccAppResource_readsBackWhatTheAPIEchoes(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set, skipping acceptance test")
	}
	projName := accName("apprb")
	appName := accName("rb")

	config := func(args, command, release string, fsGroup string) string {
		return fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %[1]q
}

resource "fpcloud_app" "test" {
  project_id      = fpcloud_project.test.id
  name            = %[2]q
  image           = "busybox:1.37"
  type            = "worker"
  command         = [%[4]s]
  args            = [%[3]s]
  release_command = [%[5]s]

  volume_mounts = [
    { source = "emptydir", mount_path = "/scratch" },
  ]

  security_context = {
    run_as_user     = 1000
    run_as_non_root = false
    %[6]s
  }
}
`, projName, appName, args, command, release, fsGroup)
	}

	var appID string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppDestroy,
		Steps: []resource.TestStep{
			{
				Config: config(`"infinity"`, `"sleep"`, `"true"`, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.test", "command.0", "sleep"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "args.0", "infinity"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "release_command.0", "true"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "volume_mounts.0.source", "emptydir"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "volume_mounts.0.mount_path", "/scratch"),
					resource.TestCheckNoResourceAttr("fpcloud_app.test", "volume_mounts.0.name"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "security_context.run_as_user", "1000"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "security_context.run_as_non_root", "false"),
					resource.TestCheckNoResourceAttr("fpcloud_app.test", "security_context.fs_group"),
					func(s *terraform.State) error {
						appID = s.RootModule().Resources["fpcloud_app.test"].Primary.ID
						return nil
					},
				),
			},
			{
				ResourceName:            "fpcloud_app.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"port", "status", "updated_at"},
			},
			{
				Config: config(`"3600"`, `"sleep"`, `"sh", "-c", "true"`, "fs_group = 1000"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.test", "args.0", "3600"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "release_command.#", "3"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "security_context.fs_group", "1000"),
				),
			},
			{
				// Changed behind Terraform's back: the plan has to see it.
				PreConfig: func() {
					c := testAccClient()
					args := []string{"7200"}
					if _, err := c.UpdateAppCommand(context.Background(), appID, nil, &args, nil); err != nil {
						t.Fatalf("out-of-band args change: %v", err)
					}
					uid := int64(2000)
					if _, err := c.SetAppSecurityContext(context.Background(), appID, &client.SecurityContext{RunAsUser: &uid}); err != nil {
						t.Fatalf("out-of-band security context change: %v", err)
					}
				},
				Config:             config(`"3600"`, `"sleep"`, `"sh", "-c", "true"`, "fs_group = 1000"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				// And the next apply converges back to the configuration.
				Config: config(`"3600"`, `"sleep"`, `"sh", "-c", "true"`, "fs_group = 1000"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.test", "args.0", "3600"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "security_context.run_as_user", "1000"),
				),
			},
		},
	})
}
