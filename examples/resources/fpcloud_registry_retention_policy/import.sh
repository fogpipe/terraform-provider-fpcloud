# Import the project-wide policy by "project_id", or a per-repo policy by
# "project_id/repo" — the repo may itself contain slashes; everything after
# the first "/" is the repo.
terraform import fpcloud_registry_retention_policy.all 6c1f0a2e-9d3b-4b7e-8f2a-0e5d7c4a1b9f
terraform import fpcloud_registry_retention_policy.api 6c1f0a2e-9d3b-4b7e-8f2a-0e5d7c4a1b9f/acme/platform/api
