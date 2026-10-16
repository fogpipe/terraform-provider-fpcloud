package provider_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// An app is created with the env and secret references it declares, before its
// first release command is gated on them (fogpipe/cloud-workspace#218, ADR-112).
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

resource "fpcloud_project_secret" "token" {
  project_id = fpcloud_project.test.id
  name       = "api-token"
  value      = "s3cr3t"
}

resource "fpcloud_app" "test" {
  project_id      = fpcloud_project.test.id
  name            = %[2]q
  image           = "busybox:1.37"
  type            = "worker"
  command         = ["sleep"]
  args            = ["infinity"]
  release_command = ["sh", "-c", "test -n \"$HOST_URL\" && test \"$API_TOKEN\" = s3cr3t && test -s \"$API_TOKEN_FILE\""]

  env = {
    HOST_URL = "https://shop.example"
  }
  secret_env = {
    API_TOKEN      = { secret = fpcloud_project_secret.token.name }
    API_TOKEN_FILE = { secret = fpcloud_project_secret.token.name, path = "/secrets/api-token" }
  }
%[3]s}
`, projName, appName, accRootImageOptOut)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppDestroy,
		Steps: []resource.TestStep{
			{
				// The release command asserts the env value, the referenced secret
				// and the file are all there; a create that wrote them afterwards
				// fails the gate and so fails the apply.
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.test", "env.HOST_URL", "https://shop.example"),
					resource.TestCheckResourceAttr("fpcloud_app.test", "secret_env.API_TOKEN_FILE.path", "/secrets/api-token"),
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
						if !seen["HOST_URL"] {
							return fmt.Errorf("config store holds %v, wanted the key the release command read", seen)
						}
						return nil
					},
				),
			},
		},
	})
}
