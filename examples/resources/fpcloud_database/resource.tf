resource "fpcloud_database" "main" {
  project = fpcloud_project.production.name # by name or id; the plan compares project_id
  name    = "maindb"
  cpu     = "1"
  memory  = "2Gi"
  storage = "20Gi"

  backup {
    enabled   = true
    schedule  = "0 3 * * *"
    retention = "30d"
  }
}
