resource "fpcloud_bucket" "assets" {
  project = fpcloud_project.production.id
  name    = "assets"

  quota_max_size    = 10737418240 # 10 GiB in bytes; reserved against the org ceiling
  quota_max_objects = 100000      # 0 = unlimited
}

# Serve a bucket as a public static website (world-readable over HTTP).
resource "fpcloud_bucket" "site" {
  project = fpcloud_project.production.id
  name    = "site"

  website_enabled        = true
  website_error_document = "index.html" # SPA fallback: serve the app shell on misses
  url_slug               = "mysite"     # optional vanity host: mysite.web.<platform domain>
}

# Hand the bucket to an app. The bucket owns two project secrets holding its
# key; referencing them is the grant, and no key value sits in this file.
resource "fpcloud_app" "uploader" {
  project_id = fpcloud_project.production.id
  name       = "uploader"
  image      = "ghcr.io/myorg/uploader:latest"

  env = {
    AWS_ENDPOINT_URL = fpcloud_bucket.assets.endpoint
    AWS_REGION       = fpcloud_bucket.assets.region
    S3_BUCKET        = fpcloud_bucket.assets.name
  }

  secret_env = {
    AWS_ACCESS_KEY_ID     = { secret = fpcloud_bucket.assets.secrets["access_key_id"] }
    AWS_SECRET_ACCESS_KEY = { secret = fpcloud_bucket.assets.secrets["secret_access_key"] }
  }
}
