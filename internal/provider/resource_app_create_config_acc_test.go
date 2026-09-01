package provider_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// An app is created with the env and secrets it declares, before its first
// release command is gated on them (fogpipe/cloud-workspace#218, ADR-112).
// Written after the create call, every value arrived after the migration that
// reads it had already run.
func TestAccAppResource_createCarriesItsConfig(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set, skipping acceptance test")
	}
	projName := accName("appcfg")
	appName := accName("cfg")

	config := fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %[1]q
}

resource "fpcloud_app" "test" {
  project_id      = fpcloud_project.test.id
  name            = %[2]q
  image           = "busybox:1.37"
  type            = "worker"
  command         = ["sleep"]
  args            = ["infinity"]
  release_command = ["sh", "-c", "test -n \"$HOST_URL\" && test -n \"$API_TOKEN\""]

  env = {
    HOST_URL = "https://shop.example"
  }
  secret = {
    API_TOKEN = "s3cr3t"
  }
%[3]s}
`, projName, appName, accRootImageOptOut)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppDestroy,
		Steps: []resource.TestStep{
			{
				// The release command asserts both values are set; a create that
				// wrote them afterwards fails the gate and so fails the apply.
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.test", "env.HOST_URL", "https://shop.example"),
					func(s *terraform.State) error {
						appID := s.RootModule().Resources["fpcloud_app.test"].Primary.ID
						cfgs, err := testAccClient().ListConfig(context.Background(), appID)
						if err != nil {
							return err
						}
						seen := map[string]bool{}
						for _, c := range cfgs {
							seen[c.Key] = true
						}
						if !seen["HOST_URL"] || !seen["API_TOKEN"] {
							return fmt.Errorf("config store holds %v, wanted both keys the release command read", seen)
						}
						return nil
					},
				),
			},
		},
	})
}
