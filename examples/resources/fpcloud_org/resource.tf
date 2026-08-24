resource "fpcloud_org" "acme" {
  name         = "acme"
  display_name = "Acme Corporation"

  # Operator-only resource ceiling, shared by every project this org owns.
  # Server-defaulted; only an operator may raise it. A project has no ceiling of
  # its own — you decide how many projects you have, so bounding each one
  # bounded a number you could raise by creating another.
  max_cpu     = "8"
  max_memory  = "16Gi"
  max_pods    = 50
  max_storage = "100Gi"
}
