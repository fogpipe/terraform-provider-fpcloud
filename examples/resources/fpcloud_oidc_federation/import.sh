# Import by "project/binding_id": the API lists trust bindings per project.
# service_account is seeded with the id the API resolved (the API never spells
# it back as an email), so reference the service account by id in the adopting
# configuration.
terraform import fpcloud_oidc_federation.ci 6c1f0a2e-9d3b-4b7e-8f2a-0e5d7c4a1b9f/2a7e4c1d-5b8f-4e3a-9c6d-1f0b8a7e5d3c
