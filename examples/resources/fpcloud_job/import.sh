# Import by job id.
terraform import fpcloud_job.sweep 0d0e5f0b-3a3f-4d9f-9a2e-3f9e1c2b4a7d

# Or declaratively (Terraform 1.5+ / OpenTofu):
#   import {
#     to = fpcloud_job.sweep
#     id = "0d0e5f0b-3a3f-4d9f-9a2e-3f9e1c2b4a7d"
#   }
