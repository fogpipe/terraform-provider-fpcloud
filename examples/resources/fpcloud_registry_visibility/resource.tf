# Make one repo anonymously pullable, without exposing the rest of the project.
resource "fpcloud_registry_visibility" "public_image" {
  project_id = fpcloud_project.example.id
  repo       = "example/public-app"
  public     = true
}
