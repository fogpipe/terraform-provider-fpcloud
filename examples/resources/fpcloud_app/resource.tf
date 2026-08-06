resource "fpcloud_app" "web" {
  project_id = fpcloud_project.production.id
  name       = "web"
  image      = "ghcr.io/myorg/webapp:latest"
  port       = 3000
  ingress    = "all"       # "all" = public, "internal" = default
  mode       = "always-on" # "always-on" (plain Deployment, default) | "serverless" (Knative)

  env = {
    APP_NAME = "My App"
    API_URL  = "https://api.example.com"
  }

  secret = {
    STRIPE_KEY     = var.stripe_key
    SESSION_SECRET = var.session_secret
  }

  # Serve /api/* publicly but keep /internal/* off the external ingress. It stays
  # reachable in-cluster, so an fpcloud_job calling it with a relative http_url
  # keeps working while outside requests are refused at the edge.
  routes = [
    {
      path       = "/internal/"
      visibility = "internal"
    },
  ]

  # All three probes default to health_check_path, which makes one request decide
  # both whether traffic arrives and whether the pod is killed. Splitting them
  # keeps a slow dependency from restarting a process that is running fine:
  # readiness checks the downstream, liveness only checks that the app responds.
  # Anything left unset here keeps the health_check_* value above.
  health_check_path = "/ready" # readiness: checks the database
  probes = {
    liveness = {
      path = "/healthz" # no downstream — a DB blip must not restart the pod
    }
    startup = {
      failure_threshold = 30 # allow 30 × period for a slow boot
    }
  }

  replicas     = 2 # fixed replica count (always-on mode)
  min_scale    = 1
  max_scale    = 5
  cpu_limit    = "500m"
  memory_limit = "512Mi"
}
