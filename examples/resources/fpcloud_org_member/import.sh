# Import by "organization_id/binding_id": the API lists members per
# organization. The email is filled from the API — the invited address while
# the member is pending, the account's address once active.
terraform import fpcloud_org_member.alice 4b9d2e7a-1c5f-4a8e-b3d6-9f0e8c7a5b1d/2a7e4c1d-5b8f-4e3a-9c6d-1f0b8a7e5d3c
