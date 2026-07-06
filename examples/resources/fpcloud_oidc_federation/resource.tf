# Trust a GitHub repo's release tags to deploy as a service account — no stored keys.
resource "fpcloud_oidc_federation" "ci" {
  project         = "my-app"
  service_account = "deployer@my-app.cloud.fogpipe.com"
  subject_pattern = "repo:my-org/my-app:ref:refs/tags/*"
  # issuer defaults to GitHub Actions; audience defaults to "fpcloud".
}
