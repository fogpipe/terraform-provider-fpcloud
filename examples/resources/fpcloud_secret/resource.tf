# A secret in the project, referenced from an app's env by name.
resource "fpcloud_secret" "stripe" {
  project_id = fpcloud_project.web.id
  name       = "stripe-key"
  value      = var.stripe_key
}

# A secret the whole organization shares: written once, referenced as
# org/<name> by an app in any of its projects.
resource "fpcloud_secret" "sentry_dsn" {
  organization_id = fpcloud_org.example.id
  name            = "sentry-dsn"
  value           = var.sentry_dsn
}

resource "fpcloud_app" "web" {
  project_id = fpcloud_project.web.id
  name       = "web"
  image      = "ghcr.io/me/web:v1"

  secret_env = {
    STRIPE_KEY = { secret = fpcloud_secret.stripe.name }
    SENTRY_DSN = { secret = "org/${fpcloud_secret.sentry_dsn.name}" }
  }
}

# A configuration that declared fpcloud_project_secret moves without a change:
moved {
  from = fpcloud_project_secret.stripe
  to   = fpcloud_secret.stripe
}
