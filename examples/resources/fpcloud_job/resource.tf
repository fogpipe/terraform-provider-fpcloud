# Call your own app's endpoint every 15 minutes. No image, nothing extra to
# deploy: the platform sends the request. `$CRON_TOKEN` is resolved from the
# app's config when the job fires, so the token lives in exactly one place, and
# a relative http_url resolves to the app's in-cluster address.
resource "fpcloud_job" "sweep" {
  project     = fpcloud_project.example.name
  name        = "sweep"
  app         = fpcloud_app.api.name
  schedule    = "*/15 * * * *"
  http_url    = "/internal/sweep"
  http_method = "POST"

  http_headers = {
    Authorization = "Bearer $CRON_TOKEN"
  }
}

# Run the app's own image with a different entrypoint, nightly in local time.
resource "fpcloud_job" "nightly_cleanup" {
  project     = fpcloud_project.example.name
  name        = "nightly-cleanup"
  app         = fpcloud_app.api.name
  schedule    = "0 3 * * *"
  timezone    = "Europe/Stockholm"
  command     = ["npm run cleanup"]
  concurrency = "forbid"
  max_retries = 3
}
