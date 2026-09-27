# A plain environment variable. A credential belongs in fpcloud_project_secret,
# mounted as a file through fpcloud_app's secret_mounts.
resource "fpcloud_app_config" "api_url" {
  app_id = fpcloud_app.web.id
  key    = "NEXT_PUBLIC_API_URL"
  value  = "https://api.example.com"
}
