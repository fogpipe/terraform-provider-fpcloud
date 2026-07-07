# Import by "project/name" (project may be a name or id), or by app id / UUID.
terraform import fpcloud_app.api web/api

# Or declaratively (Terraform 1.5+ / OpenTofu):
#   import {
#     to = fpcloud_app.api
#     id = "web/api"
#   }
