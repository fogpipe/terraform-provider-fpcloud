resource "fpcloud_domain" "main" {
  app_id = fpcloud_app.web.id
  domain = "myapp.example.com"
}

# One hostname, two apps: the API serves /api/*, the frontend serves the rest.
# Same origin, so no CORS and no cross-site cookies — but the two are separate
# images with separate deploys.
resource "fpcloud_domain" "shop" {
  app_id = fpcloud_app.web.id # serves "/" — the catch-all
  domain = "shop.example.com"

  routes = [{
    path   = "/api/"
    app_id = fpcloud_app.api.id
  }]
}
