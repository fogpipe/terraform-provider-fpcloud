# Import by "org/name", by bare name (default org), or by project id / UUID.
terraform import fpcloud_project.web acme/web
terraform import fpcloud_project.web web

# Or declaratively (Terraform 1.5+ / OpenTofu):
#   import {
#     to = fpcloud_project.web
#     id = "acme/web"
#   }
