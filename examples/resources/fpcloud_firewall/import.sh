# Import by "project/name" (project is the project's id), or by firewall id.
terraform import fpcloud_firewall.office 0d1c7e5a-8b3f-4c2e-9a6d-5f4e3b2a1c0d/office

# Or declaratively (Terraform 1.5+ / OpenTofu):
#   import {
#     to = fpcloud_firewall.office
#     id = "0d1c7e5a-8b3f-4c2e-9a6d-5f4e3b2a1c0d/office"
#   }
