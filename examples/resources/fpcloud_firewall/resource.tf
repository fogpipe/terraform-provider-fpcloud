# An allowlist: only the office and the VPN reach the app. Rules are evaluated
# in priority order, lowest first, and a request no rule matches gets
# default_action.
resource "fpcloud_firewall" "office" {
  project        = fpcloud_project.production.id
  name           = "office"
  description    = "The office and the VPN, nobody else"
  default_action = "deny"

  rules = [
    {
      priority    = 100
      action      = "allow"
      description = "Office"
      match = {
        sources = ["198.51.100.0/24", "2001:db8:100::/48"]
      }
    },
    {
      priority    = 200
      action      = "allow"
      description = "VPN exit"
      match = {
        sources = ["203.0.113.7"] # a bare address is the one-address range
      }
    },
  ]

  # The machine running this apply is outside both ranges, a CI runner for
  # instance. Without this the change is refused as one that locks its caller out.
  allow_lockout = true
}

# The firewall does nothing until something names it. It then applies to every
# host the edge serves for the app, custom domains included.
resource "fpcloud_app" "admin" {
  project_id = fpcloud_project.production.id
  name       = "admin"
  image      = "ghcr.io/myorg/admin:latest"
  ingress    = "all"

  firewall               = fpcloud_firewall.office.id
  firewall_allow_lockout = true
}

# A blocklist is the other way round: everyone is let in but the listed ranges.
resource "fpcloud_firewall" "blocklist" {
  project = fpcloud_project.production.id
  name    = "blocklist"

  rules = [
    {
      priority = 100
      action   = "deny"
      match = {
        sources = ["192.0.2.0/24"]
      }
    },
  ]
}

# The same attribute attaches one to a bucket's website.
resource "fpcloud_bucket" "docs" {
  project         = fpcloud_project.production.id
  name            = "docs"
  website_enabled = true
  firewall        = fpcloud_firewall.blocklist.id
}
