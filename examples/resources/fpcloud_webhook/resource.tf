resource "fpcloud_webhook" "autodeploy" {
  app_id        = fpcloud_app.web.id
  repo          = "myorg/webapp"
  branch        = "main"
  image_pattern = "ghcr.io/myorg/webapp:{{sha}}"
}
