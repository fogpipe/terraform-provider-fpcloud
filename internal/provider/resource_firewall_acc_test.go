package provider_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// testAccCheckFirewallDestroy verifies every fpcloud_firewall in state is gone
// from the live API after the test tears down.
func testAccCheckFirewallDestroy(s *terraform.State) error {
	c := testAccClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "fpcloud_firewall" {
			continue
		}
		_, err := c.GetFirewall(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("firewall %s still exists", rs.Primary.ID)
		}
		if !isGoneErr(err) {
			return fmt.Errorf("unexpected error checking firewall %s destroy: %w", rs.Primary.ID, err)
		}
	}
	return nil
}

// Every source here is a documentation range, and the default action stays
// allow wherever a firewall is attached, so no step can deny the address the
// test runs from.
func testAccFirewallConfig(projectName, firewallName, body string) string {
	return fmt.Sprintf(`
resource "fpcloud_project" "test" {
  name = %[1]q
}

resource "fpcloud_firewall" "test" {
  project = fpcloud_project.test.id
  name    = %[2]q
  %[3]s
}
`, projectName, firewallName, body)
}

// TestAccFirewallResource creates a firewall whose rules are written out of
// priority order with a bare address and an unmasked range, which the API
// stores reordered and canonical: the step passes only if that plans no
// change. It then edits the rules in place and round-trips the result through
// import, by id and by project and name.
func TestAccFirewallResource(t *testing.T) {
	proj := accName("fwp")
	name := accName("fw")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckFirewallDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccFirewallConfig(proj, name, `
  description    = "acceptance"
  default_action = "deny"
  rules = {
    200 = {
      action = "allow"
      match  = { sources = ["2001:DB8::/32", "192.0.2.77/24", "198.51.100.5"] }
    }
    100 = {
      action      = "deny"
      description = "one address inside the range"
      match       = { sources = ["192.0.2.13"] }
    }
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fpcloud_firewall.test", "id"),
					resource.TestCheckResourceAttr("fpcloud_firewall.test", "revision", "1"),
					resource.TestCheckResourceAttr("fpcloud_firewall.test", "default_action", "deny"),
					resource.TestCheckResourceAttr("fpcloud_firewall.test", "rules.%", "2"),
					resource.TestCheckResourceAttr("fpcloud_firewall.test", "rules.200.enabled", "true"),
					resource.TestCheckTypeSetElemAttr("fpcloud_firewall.test", "rules.200.match.sources.*", "198.51.100.5"),
					resource.TestCheckResourceAttr("fpcloud_firewall.test", "rules.100.action", "deny"),
				),
			},
			{
				// Written the way the API spells it, so the import below reads
				// back exactly this.
				Config: testAccFirewallConfig(proj, name, `
  description    = "acceptance, edited"
  default_action = "deny"
  rules = {
    100 = {
      action = "allow"
      match  = { sources = ["192.0.2.0/24", "2001:db8::/32"] }
    }
    300 = {
      action  = "allow"
      enabled = false
      match   = { sources = ["203.0.113.0/24"] }
    }
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_firewall.test", "revision", "2"),
					resource.TestCheckResourceAttr("fpcloud_firewall.test", "description", "acceptance, edited"),
					resource.TestCheckResourceAttr("fpcloud_firewall.test", "rules.%", "2"),
					resource.TestCheckResourceAttr("fpcloud_firewall.test", "rules.300.enabled", "false"),
				),
			},
			{
				ResourceName:      "fpcloud_firewall.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "fpcloud_firewall.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["fpcloud_firewall.test"]
					return rs.Primary.Attributes["project"] + "/" + rs.Primary.Attributes["name"], nil
				},
			},
		},
	})
}

// TestAccFirewallAttachToApp attaches a firewall to a real public app, edits
// its rules while attached and detaches it. Each apply returns only once the
// edge enforces the result, so a step that passes has seen the app report the
// firewall's revision as applied.
func TestAccFirewallAttachToApp(t *testing.T) {
	proj := accName("fwap")
	app := accName("fwa")
	name := accName("fw")

	config := func(source, appSettings string) string {
		return fmt.Sprintf(`
resource "fpcloud_project" "scaffold" {
  name = %[1]q
}

resource "fpcloud_firewall" "test" {
  project = fpcloud_project.scaffold.id
  name    = %[3]q
  rules = {
    100 = { action = "deny", match = { sources = [%[4]q] } }
  }
}

resource "fpcloud_app" "scaffold" {
  project_id = fpcloud_project.scaffold.id
  name       = %[2]q
  image      = "nginxinc/nginx-unprivileged:latest"
  ingress    = "all"
  %[5]s
}
`, proj, app, name, source, appSettings)
	}
	attached := "firewall = fpcloud_firewall.test.id"

	// The app's own read is what says the edge enforces the firewall.
	enforced := func(s *terraform.State) error {
		rs := s.RootModule().Resources["fpcloud_app.scaffold"]
		got, err := testAccClient().GetApp(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		fw := s.RootModule().Resources["fpcloud_firewall.test"].Primary
		if got.Firewall == nil || got.Firewall.ID != fw.ID || got.Firewall.Status != "applied" ||
			fmt.Sprint(got.Firewall.AppliedRevision) != fw.Attributes["revision"] {
			return fmt.Errorf("the app reports firewall %+v, want %s applied at revision %s", got.Firewall, fw.ID, fw.Attributes["revision"])
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			if err := testAccCheckFirewallDestroy(s); err != nil {
				return err
			}
			return testAccCheckAppDestroy(s)
		},
		Steps: []resource.TestStep{
			{
				Config: config("203.0.113.0/24", attached),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("fpcloud_app.scaffold", "firewall", "fpcloud_firewall.test", "id"),
					enforced,
				),
			},
			{
				Config: config("198.51.100.0/24", attached),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_firewall.test", "revision", "2"),
					enforced,
				),
			},
			{
				Config: config("198.51.100.0/24", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("fpcloud_app.scaffold", "firewall"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["fpcloud_app.scaffold"]
						got, err := testAccClient().GetApp(context.Background(), rs.Primary.ID)
						if err != nil {
							return err
						}
						if got.Firewall != nil {
							return fmt.Errorf("the app still reports a firewall: %+v", got.Firewall)
						}
						return nil
					},
				),
			},
		},
	})
}
