resource "fpcloud_domain" "main" {
  app_id = fpcloud_app.web.id
  domain = "myapp.example.com"
}
