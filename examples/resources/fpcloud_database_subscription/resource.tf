# Replicate into a managed database from an external PostgreSQL, so the cutover
# is a connection switch rather than a full dump and restore.
#
# Schema is NOT replicated. Load the schema (and, if you like, the existing
# rows) into the managed database first — a subscription against an empty
# database connects and moves nothing.
#
# On the source, publish first:
#   CREATE PUBLICATION fpcloud_migration FOR ALL TABLES;

resource "fpcloud_database_subscription" "migrate" {
  database_id = fpcloud_database.app.id
  name        = "migrate"
  publication = "fpcloud_migration"

  source = {
    host     = "db.abcdefghijkl.supabase.co"
    port     = 5432
    user     = "postgres"
    dbname   = "postgres"
    password = var.source_password
  }

  # The rows are already there from the dump, so replicate only what changes
  # after this point. Leaving it out copies every published table again.
  parameters = {
    copy_data = "false"
  }
}

# What the SUBSCRIBER says, which is the only place the answer exists. `applied`
# is the operand's answer to whether CREATE SUBSCRIPTION ran, and stays true on
# a subscription that has since died.
output "replication_state" {
  value = fpcloud_database_subscription.migrate.health.state
}

output "replication_lag_bytes" {
  # -1 means nobody could read a position, which is not zero lag.
  value = fpcloud_database_subscription.migrate.health.apply_lag_bytes
}
