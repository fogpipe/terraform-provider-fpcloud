resource "fpcloud_project" "production" {
  name   = "my-production-app"
  org    = "fogpipe"     # optional; defaults to the API key's organization
  egress = "restricted"  # restricted (default) | https | all
  plan   = "standard"    # starter | standard | premium
}
