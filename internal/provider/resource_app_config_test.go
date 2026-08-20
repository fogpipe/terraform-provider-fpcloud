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

func TestAccAppConfigResourceSecret(t *testing.T) {
	proj := accName("cfsp")
	app := accName("cfsa")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAppScaffold(proj, app) + `
resource "fpcloud_app_config" "secret" {
  app_id    = fpcloud_app.scaffold.id
  key       = "API_SECRET"
  value     = "super-secret-value"
  is_secret = true
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_app_config.secret", "id"),
					resource.TestCheckResourceAttr("fpcloud_app_config.secret", "is_secret", "true"),
				),
			},
			{
				// A secret's plaintext is never returned by the API, so it
				// cannot be verified on import: it arrives null and the first
				// apply re-sends the configured value (#89).
				ResourceName:            "fpcloud_app_config.secret",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"value"},
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_app_config.secret"]
					return rs.Primary.Attributes["app_id"] + "/" + rs.Primary.Attributes["key"], nil
				},
			},
		},
	})
}
