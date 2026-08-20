package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccOIDCFederationResource(t *testing.T) {
	proj := accName("oidp")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// service_account references the SA's id, not its email: import
				// seeds the attribute with the id the API resolved (Read never
				// refreshes it), so an id-spelled config is what round-trips.
				Config: fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %q
}

resource "fpcloud_service_account" "ci" {
  project_id = fpcloud_project.test.id
  name       = "ci"
}

resource "fpcloud_oidc_federation" "test" {
  project         = fpcloud_project.test.id
  subject_pattern = "repo:acme/app:ref:refs/heads/main"
  service_account = fpcloud_service_account.ci.id
}
`, proj),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_oidc_federation.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_oidc_federation.test", "audience", "fpcloud"),
				),
			},
			{
				// An existing trust binding is imported as "project/binding_id"
				// (#89). Import seeds service_account with the resolved id, so
				// with an id-spelled config nothing is ignored.
				ResourceName:      "fpcloud_oidc_federation.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_oidc_federation.test"]
					return rs.Primary.Attributes["project"] + "/" + rs.Primary.ID, nil
				},
			},
		},
	})
}
