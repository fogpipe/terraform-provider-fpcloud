package provider_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/provider"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestDatabaseResourceSchemaMutableAttrs verifies the in-place-resize surface
// (TASK-010): the granular sizing attributes exist and `version` is no longer
// RequiresReplace. Pure schema introspection — no live API needed.
func TestDatabaseResourceSchemaMutableAttrs(t *testing.T) {
	var resp fwresource.SchemaResponse
	provider.NewDatabaseResource().Schema(context.Background(), fwresource.SchemaRequest{}, &resp)
	attrs := resp.Schema.Attributes

	for _, name := range []string{"cpu", "memory", "storage", "instances", "pooler"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("fpcloud_database schema missing mutable attribute %q", name)
		}
	}

	v, ok := attrs["version"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("version attribute missing or not a StringAttribute")
	}
	for _, pm := range v.PlanModifiers {
		if strings.Contains(strings.ToLower(fmt.Sprintf("%T", pm)), "requiresreplace") {
			t.Errorf("version must be mutable in place, but has a RequiresReplace plan modifier")
		}
	}
}

func TestAccDatabaseResource(t *testing.T) {
	proj := acctest.RandomWithPrefix("tf-acc-db-proj")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %q
}

resource "fpcloud_database" "test" {
  project_id = fpcloud_project.test.id
  name       = "testdb"
}
`, proj),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_database.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_database.test", "name", "testdb"),
					resource.TestCheckResourceAttrSet("fpcloud_database.test", "status"),
				),
			},
		},
	})
}

func TestAccDatabaseResourceWithOptions(t *testing.T) {
	proj := acctest.RandomWithPrefix("tf-acc-dbo-proj")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %q
}

resource "fpcloud_database" "test" {
  project_id = fpcloud_project.test.id
  name       = "testdb"
  engine     = "postgres"
  version    = "17"
  cpu        = "1"
  memory     = "2Gi"
}
`, proj),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_database.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_database.test", "version", "17"),
				),
			},
		},
	})
}
