# Project-wide default: keep the newest 10 tags, delete anything older than 30 days.
resource "fpcloud_registry_retention_policy" "default" {
  project_id = fpcloud_project.example.id

  keep_last    = 10
  max_age_days = 30
  enabled      = true
}

# A per-repo override: this one repo keeps a longer history.
resource "fpcloud_registry_retention_policy" "releases" {
  project_id = fpcloud_project.example.id
  repo       = "example/releases"

  keep_last    = 50
  max_age_days = 0
  enabled      = true
}
