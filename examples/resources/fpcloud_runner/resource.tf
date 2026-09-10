# A project has one runner, and it scales to zero: a pod is created for a job
# and destroyed when it ends, so an idle runner costs nothing. Workflows opt in
# with `runs-on: <project>-ci`.
#
# There is no account to configure. Connect the project once with
# `fpcloud github connect` — you authorize the install as yourself, which is
# what proves you control the account — and the runner serves it.
resource "fpcloud_runner" "ci" {
  project = fpcloud_project.example.name
}

# Sized from the menu (small, medium, large), running more jobs at once, able
# to build container images and with a database beside every job.
#
# `builder` runs a rootless BuildKit alongside each job and sets BUILDKIT_HOST —
# there is no Docker daemon in a runner, and Docker-in-Docker is not available.
# It is sized apart from the runner because the two do different work, and it
# adds to what a job costs; leave its fields out to take the platform's defaults.
#
# `services` are the platform's answer to a workflow's `services:` block, which
# does not work on these runners. Each one is reachable on 127.0.0.1 from your
# steps for the life of the job.
resource "fpcloud_runner" "other" {
  project     = fpcloud_project.other.name
  size        = "large"
  max_runners = 6

  builder = {
    cpu    = "2"
    memory = "4Gi"
  }

  services = [
    {
      name  = "postgres"
      image = "postgres:18-alpine"
      env = {
        POSTGRES_PASSWORD = "ci"
      }
    },
  ]
}

# Bring your own GitHub App instead — for an organization whose policy forbids
# third-party apps, or GitHub Enterprise Server.
#
# This is the one case that names a scope: your own key says nothing about what
# it is for. Holding the key is itself the proof it is yours.
resource "fpcloud_runner" "own_app" {
  project      = fpcloud_project.isolated.name
  github_scope = "acme"

  credential                 = "app"
  github_app_id              = var.github_app_id
  github_app_installation_id = var.github_app_installation_id
  github_app_private_key     = file("${path.module}/acme-ci.private-key.pem")
}

# A runner for one repository, which is the only scope a personal GitHub
# account has: GitHub manages a personal account's runners per repository, so
# there is no account-level runner to register.
resource "fpcloud_runner" "one_repo" {
  project      = fpcloud_project.solo.name
  github_scope = "lorentzlasson/grannsnack"

  credential   = "token"
  github_token = var.github_token
}
