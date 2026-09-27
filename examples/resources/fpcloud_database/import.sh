# Import by database id. The password is returned only at creation, so an
# imported database carries an empty one — mount the owner `secret` on an app,
# or use `fpcloud db connect`, for the live credential.
terraform import fpcloud_database.events 5881262f-2d2c-4e52-9a7b-1f9c0a6f3b1d
