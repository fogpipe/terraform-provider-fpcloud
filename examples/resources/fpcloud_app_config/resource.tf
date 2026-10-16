# A plain environment variable.
resource "fpcloud_app_config" "api_url" {
  app_id = fpcloud_app.web.id
  key    = "NEXT_PUBLIC_API_URL"
  value  = "https://api.example.com"
}

# A credential: the variable references a project secret and holds its value.
resource "fpcloud_app_config" "stripe_key" {
  app_id = fpcloud_app.web.id
  key    = "STRIPE_KEY"
  secret = fpcloud_project_secret.stripe.name
}

# The same secret delivered as a file; the variable holds the path.
resource "fpcloud_app_config" "stripe_key_file" {
  app_id = fpcloud_app.web.id
  key    = "STRIPE_KEY_FILE"
  secret = fpcloud_project_secret.stripe.name
  path   = "/secrets/stripe"
}
