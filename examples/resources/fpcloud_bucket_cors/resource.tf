# Let a page on your own domain upload straight to the bucket.
#
# Without this the upload fails before it is sent: the browser asks the bucket
# first, with an OPTIONS preflight, and that preflight carries no signature — so
# a correctly presigned PUT is refused by the browser rather than by the store,
# and reported as a CORS error rather than as anything about permissions.
resource "fpcloud_bucket_cors" "photos" {
  bucket_id = fpcloud_bucket.photos.id

  rule = [{
    allowed_origins = ["https://example.com"]
    allowed_methods = ["GET", "PUT", "HEAD"]
    allowed_headers = ["*"]

    # Without ETag the upload succeeds and the page cannot read what it
    # uploaded. It is the single most common way a correct-looking rule fails.
    expose_headers  = ["ETag"]
    max_age_seconds = 3600
  }]
}

# Order is significant — the store answers with the FIRST rule whose origin
# matches, so the narrow rule has to come before the broad one. Reversed, every
# origin would match the wildcard and nothing would ever reach the rule below it.
resource "fpcloud_bucket_cors" "downloads" {
  bucket_id = fpcloud_bucket.downloads.id

  rule = [
    {
      allowed_origins = ["https://app.example.com"]
      allowed_methods = ["GET", "PUT", "DELETE"]
      allowed_headers = ["*"]
      expose_headers  = ["ETag"]
    },
    {
      allowed_origins = ["*"]
      allowed_methods = ["GET"]
    },
  ]
}
