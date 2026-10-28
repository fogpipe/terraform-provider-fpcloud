# Import by "project/<project_id>/<binding_id>" or "organization/<org_id>/<binding_id>":
# the API lists bindings per scope. The member comes back spelled the way the
# server canonicalized it, and the resource it is held on, if any, is read back.
terraform import fpcloud_iam_binding.viewer project/6c1f0a2e-9d3b-4b7e-8f2a-0e5d7c4a1b9f/2a7e4c1d-5b8f-4e3a-9c6d-1f0b8a7e5d3c
terraform import fpcloud_iam_binding.finance organization/4b9d2e7a-1c5f-4a8e-b3d6-9f0e8c7a5b1d/9e1c3b5a-7d2f-4c8e-a6b4-0f3d9e8c7b2a
