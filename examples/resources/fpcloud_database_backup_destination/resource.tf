# Off-site replication of a managed database's backups to a customer-owned
# S3-compatible bucket (Cloudflare R2 here), in addition to the platform-managed
# backup already configured on fpcloud_database.
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

resource "fpcloud_database_backup_destination" "offsite" {
  database_id = fpcloud_database.main.id

  provider_name     = "s3"
  bucket            = "maindb-backups"
  endpoint          = "https://<account>.r2.cloudflarestorage.com"
  region            = "auto"
  access_key_id     = var.r2_access_key_id
  secret_access_key = var.r2_secret_access_key
  schedule          = "0 4 * * *"
}

# AWS, keyless via OIDC federation (no stored secret):
#
# resource "fpcloud_database_backup_destination" "offsite" {
#   database_id   = fpcloud_database.main.id
#   provider_name = "aws"
#   bucket        = "maindb-backups"
#   role_arn      = "arn:aws:iam::123456789012:role/fpcloud-backup"
# }
