# CI for the project's GitHub account. The pool scales to zero: a pod is created
# for a job and destroyed when it ends, so an idle pool costs nothing. Workflows
# opt in with `runs-on: ci`.
#
# There is no account to configure. Connect the project once with
# `fpcloud github connect` — you authorize the install as yourself, which is
# what proves you control the account — and every pool serves it.
resource "fpcloud_runner" "ci" {
  project     = fpcloud_project.example.name
  name        = "ci"
  max_runners = 4
}

# A second pool, able to build container images. `builder` runs a rootless
# BuildKit alongside each job and sets BUILDKIT_HOST — there is no Docker daemon
# in a runner, and Docker-in-Docker is not available.
#
# `cpu`/`memory` bound the runner your steps execute in; the builder is sized
# apart from it because the two do different work, and it adds to what the pool
# costs. Leave the builder's fields out to take the platform's defaults.
resource "fpcloud_runner" "shared" {
  project     = fpcloud_project.example.name
  name        = "shared"
  min_runners = 1
  max_runners = 6
  cpu         = "4"
  memory      = "8Gi"

  builder = {
    cpu    = "2"
    memory = "4Gi"
  }
}

# Bring your own GitHub App instead — for an organization whose policy forbids
# third-party apps, or GitHub Enterprise Server.
#
# This is the one case that names an account: your own key says nothing about
# which account it is for. Holding the key is itself the proof it is yours.
resource "fpcloud_runner" "own_app" {
  project        = fpcloud_project.example.name
  name           = "isolated"
  github_account = "acme"

  credential                 = "app"
  github_app_id              = var.github_app_id
  github_app_installation_id = var.github_app_installation_id
  github_app_private_key     = file("${path.module}/acme-ci.private-key.pem")
}
