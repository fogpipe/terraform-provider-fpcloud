resource "fpcloud_project" "production" {
  name         = "my-production-app"
  display_name = "My Production App" # optional; mutable cosmetic label
  org          = "fogpipe"           # optional; uuid, opaque id or readable name — defaults to the API key's organization
  egress       = "restricted"        # restricted (default) | https | all (tcp 25/465 never leave)
}
