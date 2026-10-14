package provider_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/provider"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestDatabaseResourceSchemaMutableAttrs verifies the in-place-resize surface
// (TASK-010): the granular sizing attributes exist and `version` is no longer
// RequiresReplace. Pure schema introspection — no live API needed.
func TestDatabaseResourceSchemaMutableAttrs(t *testing.T) {
	var resp fwresource.SchemaResponse
	provider.NewDatabaseResource().Schema(context.Background(), fwresource.SchemaRequest{}, &resp)
	attrs := resp.Schema.Attributes

	for _, name := range []string{"cpu", "memory", "storage", "pooler"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("fpcloud_database schema missing mutable attribute %q", name)
		}
	}

	// instances is reported and never set (ADR-136). A client-side default is
	// what would plan a replica away on the next apply, so the attribute must
	// carry neither a default nor a way to configure one.
	inst, ok := attrs["instances"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("instances attribute missing or not an Int64Attribute")
	}
	if inst.Optional || inst.Required {
		t.Errorf("instances must be computed-only, but is configurable")
	}
	if inst.Default != nil {
		t.Errorf("instances must carry no client-side default")
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

// A default here is not a convenience, it is an override: a config that names
// no version plans the provider's number instead of the server's, so the
// platform's default can move and every Terraform-declared database stays on
// the old major (#883).
func TestDatabaseVersionTakesThePlatformDefault(t *testing.T) {
	var resp fwresource.SchemaResponse
	provider.NewDatabaseResource().Schema(context.Background(), fwresource.SchemaRequest{}, &resp)

	v, ok := resp.Schema.Attributes["version"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("version attribute missing or not a StringAttribute")
	}
	if v.Default != nil {
		t.Errorf("version pins a provider-side default; the server states the major")
	}
	if !v.Computed || !v.Optional {
		t.Errorf("version must be Optional+Computed so the server can state it")
	}
}

func TestAccDatabaseResource(t *testing.T) {
	proj := accName("dbp")

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
			{
				// An existing database can be brought under Terraform by id
				// (fogpipe/cloud-workspace#32). The resource carries no credential
				// (ADR-281), so the import is whole; status moves as the cluster
				// provisions.
				ResourceName:            "fpcloud_database.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"status"},
			},
		},
	})
}

func TestAccDatabaseResourceWithOptions(t *testing.T) {
	proj := accName("dbop")

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

// A rotation is asked for by changing password_rotation, adding it included;
// removing it, or leaving it as it was, rotates nothing
// (fogpipe/cloud-workspace#297).
func TestRotationRequested(t *testing.T) {
	cases := []struct {
		name       string
		prev, next types.String
		want       bool
	}{
		{"added", types.StringNull(), types.StringValue("2026-09-07"), true},
		{"changed", types.StringValue("1"), types.StringValue("2"), true},
		{"unchanged", types.StringValue("1"), types.StringValue("1"), false},
		{"removed", types.StringValue("1"), types.StringNull(), false},
		{"emptied", types.StringValue("1"), types.StringValue(""), false},
		{"never set", types.StringNull(), types.StringNull(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := provider.RotationRequested(tc.prev, tc.next); got != tc.want {
				t.Errorf("rotationRequested(%v, %v) = %v, want %v", tc.prev, tc.next, got, tc.want)
			}
		})
	}
}

// The resource carries no credential (ADR-281): rotation is still asked for
// through `password_rotation`, and nothing it issues reaches state.
func TestTheResourceCarriesNoCredential(t *testing.T) {
	var resp fwresource.SchemaResponse
	provider.NewDatabaseResource().Schema(context.Background(), fwresource.SchemaRequest{}, &resp)
	for _, name := range []string{"password", "username"} {
		if _, found := resp.Schema.Attributes[name]; found {
			t.Errorf("%s is an attribute again; a credential is read on purpose, never carried in state", name)
		}
	}
	r, ok := resp.Schema.Attributes["password_rotation"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("password_rotation attribute missing or not a StringAttribute")
	}
	if !r.Optional || r.Computed {
		t.Errorf("password_rotation is the practitioner's request and must be Optional only")
	}
}
