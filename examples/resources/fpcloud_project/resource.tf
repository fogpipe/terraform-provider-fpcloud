resource "fpcloud_project" "production" {
  name         = "my-production-app"
  display_name = "My Production App" # optional; mutable cosmetic label
  org          = "fogpipe"           # optional; defaults to the API key's organization
  egress       = "restricted"        # restricted (default) | https | all
}
