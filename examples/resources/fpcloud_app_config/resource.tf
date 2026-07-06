# Plain environment variable
resource "fpcloud_app_config" "api_url" {
  app_id = fpcloud_app.web.id
  key    = "NEXT_PUBLIC_API_URL"
  value  = "https://api.example.com"
}

# Secret (encrypted at rest)
resource "fpcloud_app_config" "stripe_key" {
  app_id    = fpcloud_app.web.id
  key       = "STRIPE_SECRET_KEY"
  value     = var.stripe_secret_key
  is_secret = true
}
