# A long-running process with no port, URL or health check: a queue consumer.
# It keeps everything else an app has (config, secrets, identity, logs) and
# runs the same image as the web app with a different entrypoint.
resource "fpcloud_app" "consumer" {
  project_id = fpcloud_project.production.id
  name       = "consumer"
  image      = "ghcr.io/myorg/api:1.4.2"
  type       = "worker"
  command    = ["node", "worker.js"]

  secret_env = {
    DATABASE_URL = { secret = fpcloud_database.orders.secrets["url"] }
  }
}
