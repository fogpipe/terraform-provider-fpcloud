# Import by organization name (or by organization id / UUID).
terraform import fpcloud_org.acme acme

# Or declaratively (Terraform 1.5+ / OpenTofu):
#   import {
#     to = fpcloud_org.acme
#     id = "acme"
#   }
