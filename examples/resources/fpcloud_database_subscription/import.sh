# Import by "<database_id>/<name>".
#
# The source password is write-only and imports as null, so the first apply
# after an import replaces the subscription: there is no way to import a
# credential nothing returns.
terraform import fpcloud_database_subscription.migrate 5d8a3f2e-9c1b-4e7a-a4d6-0f9b7c5e3a1d/migrate
