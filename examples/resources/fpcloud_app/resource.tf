resource "fpcloud_app" "web" {
  project_id = fpcloud_project.production.id
  name       = "web"
  image      = "ghcr.io/myorg/webapp:latest"
  port       = 3000
  ingress    = "all"        # "all" = public, "internal" = default
  mode       = "always-on"  # "always-on" (plain Deployment, default) | "serverless" (Knative)

  env = {
    APP_NAME = "My App"
    API_URL  = "https://api.example.com"
  }

  secret = {
    STRIPE_KEY     = var.stripe_key
    SESSION_SECRET = var.session_secret
  }

  replicas     = 2 # fixed replica count (always-on mode)
  min_scale    = 1
  max_scale    = 5
  cpu_limit    = "500m"
  memory_limit = "512Mi"
}
