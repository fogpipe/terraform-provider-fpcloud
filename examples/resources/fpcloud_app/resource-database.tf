# An app handed a managed database. Referencing the database's owner secrets is
# the grant: the app's identity is handed the database with the reference, and
# removing the reference revokes it. No password ever sits in this file or in
# state.
resource "fpcloud_database" "orders" {
  project_id = fpcloud_project.production.id
  name       = "orders"
  pooler     = true
}

resource "fpcloud_app" "api" {
  project_id = fpcloud_project.production.id
  name       = "api"
  image      = "ghcr.io/myorg/api:1.4.2"
  port       = 8080

  secret_env = {
    DATABASE_URL = { secret = fpcloud_database.orders.secrets["url"] }
    PGPASSWORD   = { secret = fpcloud_database.orders.secrets["password"] }
  }

  env = {
    # The read replica is the same credential at read_host: compose the URL
    # from the password the reference above delivers. Reads here can be stale.
    DATABASE_READ_URL = "postgres://app:$(PGPASSWORD)@${fpcloud_database.orders.read_host}:${fpcloud_database.orders.port}/app?sslmode=require"
  }

  # Runs from the image being deployed, with the env above, before the new
  # version goes live. A failed migration aborts the deploy.
  release_command = ["npm", "run", "migrate"]
}
