package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAppConfigResource(t *testing.T) {
	proj := acctest.RandomWithPrefix("tf-acc-cfg-proj")
	app := acctest.RandomWithPrefix("tf-acc-cfg-app")

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
		},
	})
}

func TestAccAppConfigResourceSecret(t *testing.T) {
	proj := acctest.RandomWithPrefix("tf-acc-cfgs-proj")
	app := acctest.RandomWithPrefix("tf-acc-cfgs-app")

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
		},
	})
}
