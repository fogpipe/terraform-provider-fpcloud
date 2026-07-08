resource "fpcloud_database" "main" {
  project_id = fpcloud_project.production.id
  name       = "maindb"
  version    = "17"
  cpu        = "1"
  memory     = "2Gi"
  storage    = "20Gi"

  backup {
    enabled   = true
    schedule  = "0 3 * * *"
    retention = "30d"
  }
}
