resource "fpcloud_project" "production" {
  org  = fpcloud_org.acme.id
  name = "production"
}

resource "fpcloud_bucket" "assets" {
  project = fpcloud_project.production.id
  name    = "assets"
}

resource "fpcloud_app" "uploader" {
  project_id = fpcloud_project.production.id
  name       = "uploader"
  image      = "ghcr.io/myorg/uploader:latest"
}

# Bind the bucket to the app. Fogpipe mints a scoped S3 key and injects the
# S3_ENDPOINT / S3_REGION / S3_BUCKET / S3_ACCESS_KEY_ID / S3_SECRET_ACCESS_KEY
# (and AWS_* aliases) into the app's pod via a k8s Secret — no need to wire the
# credentials into env/secret by hand. Changing read_only rebinds.
resource "fpcloud_app_bucket" "uploader_assets" {
  app_id    = fpcloud_app.uploader.id
  bucket_id = fpcloud_bucket.assets.id
  read_only = false
}
