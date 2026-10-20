# Every app is its own service account, created and deleted with it, holding
# nothing until granted. Grant it with fpcloud_iam_binding: a project role for
# calling the Fogpipe API from its pods, or a resource handed over directly.
# Inside its pods the app proves who it is with a token the cluster rotates, so
# there is no key to mount, rotate or revoke.
resource "fpcloud_app" "reporter" {
  project_id = fpcloud_project.production.id
  name       = "reporter"
  image      = "ghcr.io/myorg/reporter:latest"
}

# Read the project through the API, as itself.
resource "fpcloud_iam_binding" "reporter_viewer" {
  project_id  = fpcloud_project.production.id
  member_type = "serviceAccount"
  member_id   = fpcloud_app.reporter.service_account
  role        = "viewer"
}

# Read one secret's value through the API, without referencing it in env.
resource "fpcloud_iam_binding" "reporter_reads_stripe" {
  project_id  = fpcloud_project.production.id
  resource    = "secret/${fpcloud_secret.stripe.id}"
  member_type = "serviceAccount"
  member_id   = fpcloud_app.reporter.service_account
  role        = "reader"
}
