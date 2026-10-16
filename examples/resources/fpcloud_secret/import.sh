# Import by id. The value is write-only, so the first apply after an import
# writes the configuration's value, which rolls the apps referencing it.
terraform import fpcloud_secret.stripe 4b9d2e7a-1c5f-4a8e-b3d6-9f0e8c7a5b1d
