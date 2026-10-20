# An app handed a bucket. The bucket owns two project secrets holding its key;
# referencing them is the grant. Endpoint, region and bucket name are plain
# config, so any S3 client reads the standard AWS_* variables and nothing else.
resource "fpcloud_bucket" "uploads" {
  project = fpcloud_project.production.id
  name    = "uploads"
}

resource "fpcloud_app" "uploader" {
  project_id = fpcloud_project.production.id
  name       = "uploader"
  image      = "ghcr.io/myorg/uploader:latest"

  env = {
    AWS_ENDPOINT_URL = fpcloud_bucket.uploads.endpoint
    AWS_REGION       = fpcloud_bucket.uploads.region
    S3_BUCKET        = fpcloud_bucket.uploads.name
  }

  secret_env = {
    AWS_ACCESS_KEY_ID     = { secret = fpcloud_bucket.uploads.secrets["access_key_id"] }
    AWS_SECRET_ACCESS_KEY = { secret = fpcloud_bucket.uploads.secrets["secret_access_key"] }
  }
}
