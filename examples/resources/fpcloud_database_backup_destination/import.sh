# Import by the database id alone: a database has at most one backup
# destination. secret_access_key is write-only and imports as null — the first
# apply re-sends the configured value.
terraform import fpcloud_database_backup_destination.offsite 5d8a3f2e-9c1b-4e7a-a4d6-0f9b7c5e3a1d
