# A scoped, read-only S3 access key for a bucket. The secret access key is only
# returned once, at creation — Terraform keeps it in state (marked sensitive).
resource "fpcloud_bucket_key" "readonly" {
  bucket_id = fpcloud_bucket.assets.id
  name      = "cdn-reader"
  read      = true
  write     = false
  owner     = false
}
