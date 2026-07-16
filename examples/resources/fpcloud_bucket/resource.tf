resource "fpcloud_bucket" "assets" {
  project = fpcloud_project.production.id
  name    = "assets"

  quota_max_size    = 10737418240 # 10 GiB in bytes (0 = unlimited)
  quota_max_objects = 100000      # 0 = unlimited
}

# Wire the bucket's S3 credentials into an app. The secret access key is only
# returned once, at creation — Terraform keeps it in state (marked sensitive).
resource "fpcloud_app" "uploader" {
  project_id = fpcloud_project.production.id
  name       = "uploader"
  image      = "ghcr.io/myorg/uploader:latest"

  env = {
    S3_ENDPOINT = fpcloud_bucket.assets.endpoint
    S3_REGION   = fpcloud_bucket.assets.region
    S3_BUCKET   = fpcloud_bucket.assets.name
  }

  secret = {
    S3_ACCESS_KEY_ID     = fpcloud_bucket.assets.access_key_id
    S3_SECRET_ACCESS_KEY = fpcloud_bucket.assets.secret_access_key
  }
}
