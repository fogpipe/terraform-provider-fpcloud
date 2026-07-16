resource "fpcloud_project" "production" {
  name         = "my-production-app"
  display_name = "My Production App" # optional; mutable cosmetic label
  org          = "fogpipe"           # optional; defaults to the API key's organization
  egress       = "restricted"        # restricted (default) | https | all

  # Operator-only resource caps (namespace ResourceQuota). Server-defaulted;
  # only an operator or org owner may raise them.
  max_cpu     = "4"
  max_memory  = "8Gi"
  max_pods    = 50
  max_storage = "100Gi"
}
