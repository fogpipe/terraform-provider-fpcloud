resource "fpcloud_database" "main" {
  project_id = fpcloud_project.production.id
  name       = "maindb"
  version    = "17"
  plan       = "standard"

  backup {
    enabled   = true
    schedule  = "0 3 * * *"
    retention = "30d"
  }
}
