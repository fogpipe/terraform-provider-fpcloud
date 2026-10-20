# A person on a project. Roles contain each other: viewer < editor < owner.
resource "fpcloud_iam_binding" "jane_editor" {
  project_id  = fpcloud_project.production.id
  member_type = "user"
  member_id   = "jane@example.com"
  role        = "editor"
}

# A resource handed to an app's own identity, with no secret reference in the
# app's env: the binding alone is the grant. The role is the kind's handed
# role, `user` on a database or a bucket, `reader` on a secret.
resource "fpcloud_iam_binding" "api_uses_orders" {
  project_id  = fpcloud_project.production.id
  resource    = "database/${fpcloud_database.orders.id}"
  member_type = "serviceAccount"
  member_id   = fpcloud_app.api.service_account
  role        = "user"
}
