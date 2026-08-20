# Import by "app_id/key". A non-secret entry imports completely; a secret's
# plaintext is never returned by the API, so its value arrives null and the
# first apply re-sends the configured value in place.
terraform import fpcloud_app_config.database_url 9f3e6b1d-7a2c-4d8f-b5e9-1c0a8d6f4b2e/DATABASE_URL
