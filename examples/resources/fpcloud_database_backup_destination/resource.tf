# Static-key S3-compatible store (Cloudflare R2, Backblaze B2, Hetzner, Garage)
resource "fpcloud_database_backup_destination" "r2" {
  database_id       = fpcloud_database.main.id
  provider_type     = "s3"
  endpoint          = "https://<account-id>.r2.cloudflarestorage.com"
  region            = "auto"
  bucket            = "my-db-backups"
  access_key_id     = var.r2_access_key_id
  secret_access_key = var.r2_secret_access_key
  schedule          = "0 4 * * *" # optional; omit for on-demand only
}

# Keyless AWS (IAM role assumed via web identity)
resource "fpcloud_database_backup_destination" "aws" {
  database_id   = fpcloud_database.main.id
  provider_type = "aws"
  bucket        = "my-db-backups"
  region        = "eu-central-1"
  role_arn      = "arn:aws:iam::123456789012:role/fpcloud-backup"
}
