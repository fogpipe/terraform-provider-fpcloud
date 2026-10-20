# A standalone identity in a project: a deployer that is no app. An app's own
# identity is never declared here; it is read from the app's `service_account`.
resource "fpcloud_service_account" "deployer" {
  project_id   = fpcloud_project.production.id
  name         = "deployer"
  display_name = "GitHub Actions deployer"
}

resource "fpcloud_iam_binding" "deployer_editor" {
  project_id  = fpcloud_project.production.id
  member_type = "serviceAccount"
  member_id   = fpcloud_service_account.deployer.email
  role        = "editor"
}

# An organization identity that outlives any one project: the account a
# Terraform root runs as, which creates and destroys projects of its own.
resource "fpcloud_service_account" "tofu" {
  organization_id = fpcloud_org.example.id
  name            = "tofu"
}
