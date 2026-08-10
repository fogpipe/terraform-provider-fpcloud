# CI for one repository. The pool scales to zero: a pod is created for a job and
# destroyed when it ends, so an idle pool costs nothing. Workflows opt in with
# `runs-on: ci`.
#
# No credential is configured: the default `platform` uses the Fogpipe GitHub
# App, which you install on your account once, in one click. The first apply
# fails with the install link if it is not installed yet.
resource "fpcloud_runner" "ci" {
  project           = fpcloud_project.example.name
  name              = "ci"
  github_config_url = "https://github.com/acme/api"
  max_runners       = 4
}

# A shared pool for the whole organization, able to build container images.
# `builds` runs a rootless BuildKit alongside each job and sets BUILDKIT_HOST —
# there is no Docker daemon in a runner, and Docker-in-Docker is not available.
resource "fpcloud_runner" "shared" {
  project           = fpcloud_project.example.name
  name              = "shared"
  github_config_url = "https://github.com/acme"
  min_runners       = 1
  max_runners       = 6
  builds            = true
  cpu               = "4"
  memory            = "8Gi"
}

# Bring your own GitHub App instead — for an organization whose policy forbids
# third-party apps, or GitHub Enterprise Server.
resource "fpcloud_runner" "own_app" {
  project           = fpcloud_project.example.name
  name              = "isolated"
  github_config_url = "https://github.com/acme/api"

  credential                 = "app"
  github_app_id              = var.github_app_id
  github_app_installation_id = var.github_app_installation_id
  github_app_private_key     = file("${path.module}/acme-ci.private-key.pem")
}
