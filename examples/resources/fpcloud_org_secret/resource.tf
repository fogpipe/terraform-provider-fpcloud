# An org-scoped secret bundle, mirrored as a k8s Secret into two projects.
resource "fpcloud_org_secret" "shared_creds" {
  org_id = fpcloud_org.example.id
  name   = "shared-creds"

  data = {
    API_KEY    = var.upstream_api_key
    API_SECRET = var.upstream_api_secret
  }

  targets = [
    fpcloud_project.web.id,
    fpcloud_project.worker.id,
  ]
}
