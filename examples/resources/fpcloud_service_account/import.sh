# Import by "project/<project_id>/<service_account_id>" for a project-scoped
# machine identity, or "org/<organization_id>/<service_account_id>" for one the
# organization holds directly. The API lists service accounts per owner and has
# no get-by-id, so the import id names which owner to look in.
terraform import fpcloud_service_account.deployer project/6c1f0a2e-9d3b-4b7e-8f2a-0e5d7c4a1b9f/2a7e4c1d-5b8f-4e3a-9c6d-1f0b8a7e5d3c

terraform import fpcloud_service_account.ci org/8e2b1f6a-3c7d-4a9e-b1f5-7d2c0a6e4b8f/9f3a5c7e-2d1b-4f8a-a6c3-5e0b9d7f2a4c
