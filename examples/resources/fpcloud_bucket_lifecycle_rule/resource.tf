# Expire generated renders after 30 days. Objects under the prefix are deleted
# by age alone — a render nobody has touched in 30 days ages out even though it
# is still the current one, which is fine here because a miss re-renders it.
resource "fpcloud_bucket_lifecycle_rule" "renders" {
  bucket_id   = fpcloud_bucket.assets.id
  prefix      = "renders/"
  expire_days = 30
}

# User uploads in the same bucket are never expired — they only reclaim the
# parts of multipart uploads that were abandoned. Rules are keyed by prefix, so
# this one and the rule above coexist on one bucket.
resource "fpcloud_bucket_lifecycle_rule" "uploads" {
  bucket_id                    = fpcloud_bucket.assets.id
  prefix                       = "uploads/"
  abort_incomplete_upload_days = 7
}
