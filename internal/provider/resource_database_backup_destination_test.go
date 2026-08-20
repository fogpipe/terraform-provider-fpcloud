package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDatabaseBackupDestinationResource(t *testing.T) {
	proj := accName("bdst")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// s3 is the one provider type whose required fields need no
				// real cloud account — the server validates shape, not reach.
				Config: fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %q
}

resource "fpcloud_database" "test" {
  project_id = fpcloud_project.test.id
  name       = "backupdb"
}

resource "fpcloud_database_backup_destination" "test" {
  database_id       = fpcloud_database.test.id
  provider_type     = "s3"
  bucket            = "tfa-backups"
  endpoint          = "https://s3.example.com"
  access_key_id     = "AKIAEXAMPLE"
  secret_access_key = "not-a-real-secret"
}
`, proj),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_database_backup_destination.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_database_backup_destination.test", "enabled", "true"),
				),
			},
			{
				// The destination is imported by its database's id — a database
				// has at most one, and that id is also this resource's (#89).
				// secret_access_key is write-only: the API never returns it, so
				// it imports as null and the first apply re-sends it.
				ResourceName:            "fpcloud_database_backup_destination.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"secret_access_key"},
			},
		},
	})
}
