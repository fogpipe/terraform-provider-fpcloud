# Scale to zero between requests. A serverless app has no fixed replica count:
# min_scale 0 lets it sleep, max_scale bounds a burst. Switching mode later
# moves the running app over in place.
resource "fpcloud_app" "preview" {
  project_id = fpcloud_project.production.id
  name       = "preview"
  image      = "ghcr.io/myorg/preview:latest"
  port       = 3000
  ingress    = "all"
  mode       = "serverless"
  min_scale  = 0
  max_scale  = 10
}
