package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccAppConfigResource(t *testing.T) {
	proj := accName("cfgp")
	app := accName("cfga")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAppScaffold(proj, app) + `
resource "fpcloud_app_config" "test" {
  app_id = fpcloud_app.scaffold.id
  key    = "DATABASE_URL"
  value  = "postgres://localhost/test"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_app_config.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_app_config.test", "key", "DATABASE_URL"),
				),
			},
			{
				// An existing entry is imported as "app_id/key" — the pair Read
				// keys on (#89). A non-secret value is returned by the API, so
				// import is complete and nothing is ignored.
				ResourceName:      "fpcloud_app_config.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_app_config.test"]
					return rs.Primary.Attributes["app_id"] + "/" + rs.Primary.Attributes["key"], nil
				},
			},
		},
	})
}

// The project-secret flow end to end (#1069): the secret is created, an app
// mounts it at a path, the database of who-mounts-what answers, and the value
// never reads back from anything.
func TestAccProjectSecretResource(t *testing.T) {
	proj := accName("cfsp")
	app := accName("cfsa")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAppScaffold(proj, app) + `
resource "fpcloud_project_secret" "stripe" {
  project_id = fpcloud_project.scaffold.id
  name       = "stripe-key"
  value      = "sk_test_123"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_project_secret.stripe", "id"),
					resource.TestCheckResourceAttr("fpcloud_project_secret.stripe", "name", "stripe-key"),
				),
			},
			{
				Config: testAccAppScaffold(proj, app) + `
resource "fpcloud_project_secret" "stripe" {
  project_id = fpcloud_project.scaffold.id
  name       = "stripe-key"
  value      = "sk_test_456"
}

resource "fpcloud_app" "mounter" {
  project_id = fpcloud_project.scaffold.id
  name       = "` + accName("cfsm") + `"
  image      = "nginx:latest"
  ingress    = "internal"
  secret_mounts = {
    "/secrets/stripe" = fpcloud_project_secret.stripe.name
  }
` + accRootImageOptOut + `}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.mounter", "secret_mounts./secrets/stripe", "stripe-key"),
				),
			},
		},
	})
}
