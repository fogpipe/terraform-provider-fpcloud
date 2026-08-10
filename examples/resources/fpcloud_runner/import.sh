# Import by runner id. The credential is write-only, so it is not read back —
# put it in the config before the first apply after an import, or the next apply
# will send an empty one.
terraform import fpcloud_runner.ci 0d0e5f0b-3a3f-4d9f-9a2e-3f9e1c2b4a7d

# Or declaratively (Terraform 1.5+ / OpenTofu):
#   import {
#     to = fpcloud_runner.ci
#     id = "0d0e5f0b-3a3f-4d9f-9a2e-3f9e1c2b4a7d"
#   }
