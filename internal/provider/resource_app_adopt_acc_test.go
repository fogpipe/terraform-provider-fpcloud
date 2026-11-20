package provider_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// An app that already exists is adopted into state by a create with
// adopt_existing, and it is the same app: the provider takes it over on the
// API's ALREADY_EXISTS answer and on nothing else, so an adoption that reads
// any other conflict as "exists" fails here instead of in someone's apply.
func TestAccAppResource_adoptExisting(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set, skipping acceptance test")
	}
	projName := accName("adopt")
	appName := accName("adp")

	project := fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %q
}
`, projName)
	adopting := project + fmt.Sprintf(`
resource "fpcloud_app" "test" {
  project        = fpcloud_project.test.id
  name           = %[1]q
  image          = "busybox:1.37"
  type           = "worker"
  command        = ["sleep"]
  args           = ["infinity"]
  adopt_existing = true
%[2]s}
`, appName, accRootImageOptOut)

	var projectID, existingID string
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppDestroy,
		Steps: []resource.TestStep{
			{
				Config: project,
				Check: func(s *terraform.State) error {
					projectID = s.RootModule().Resources["fpcloud_project.test"].Primary.ID
					return nil
				},
			},
			{
				// The app is made outside this configuration, as a console or
				// another state would make it, and the create meets it.
				PreConfig: func() {
					uid, root := int64(1000), false
					app, err := testAccClient().CreateApp(context.Background(), projectID, client.CreateAppRequest{
						Name:            appName,
						Image:           "busybox:1.37",
						Type:            "worker",
						Command:         []string{"sleep"},
						Args:            []string{"infinity"},
						SecurityContext: &client.SecurityContext{RunAsUser: &uid, RunAsNonRoot: root},
					})
					if err != nil {
						t.Fatalf("create the app to adopt: %v", err)
					}
					existingID = app.ID
				},
				Config: adopting,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_app.test", "name", appName),
					func(s *terraform.State) error {
						if got := s.RootModule().Resources["fpcloud_app.test"].Primary.ID; got != existingID {
							return fmt.Errorf("state holds app %s, want the existing app %s adopted", got, existingID)
						}
						return nil
					},
				),
			},
		},
	})
}
