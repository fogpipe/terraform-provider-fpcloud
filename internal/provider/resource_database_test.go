package provider_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/provider"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "fpcloud_database" "test" {
						project_id = "test-project"
						name       = "testdb"
					}
				`,
			},
		},
	})
}

func TestAccDatabaseResourceWithOptions(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "fpcloud_database" "test" {
						project_id = "test-project"
						name       = "testdb"
						engine     = "postgres"
						version    = "17"
						cpu        = "1"
						memory     = "2Gi"
					}
				`,
			},
		},
	})
}
