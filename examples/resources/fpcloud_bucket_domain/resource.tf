# A bucket served as a public static website with a custom domain.
resource "fpcloud_bucket" "site" {
  project = fpcloud_project.production.id
  name    = "site"

  website_enabled        = true
  website_index_document = "index.html"
  website_error_document = "404.html"
}

resource "fpcloud_bucket_domain" "www" {
  bucket_id = fpcloud_bucket.site.id
  domain    = "www.example.com"
}

# The domain starts in pending_verification. Add at your DNS provider:
#   TXT  _fpcloud-challenge.www.example.com  fpcloud-verify=<token>
#   CNAME www.example.com -> <platform domain>   (A record for an apex)
# then the platform serves it and TLS issues automatically.
output "site_url" {
  value = fpcloud_bucket.site.website_url
}
