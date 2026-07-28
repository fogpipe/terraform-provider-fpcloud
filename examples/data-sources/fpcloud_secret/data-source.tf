# Read an org secret bundle and wire it into an app, without ever putting the
# values in the configuration.
data "fpcloud_secret" "backend" {
  org_id = var.org_id
  name   = "argus-backend"
}

resource "fpcloud_app" "api" {
  project = fpcloud_project.demo.name
  name    = "api"
  image   = "registry.cloud.fogpipe.com/acme/demo/api:v1"

  # Multi-line values (a PEM, an APNs .p8 key) survive intact here; the old
  # `fpcloud secrets get | jq > TF_VAR_*` workaround could not carry them.
  secret = data.fpcloud_secret.backend.data
}
