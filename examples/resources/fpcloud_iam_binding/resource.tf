# A person on a project. A member may hold several roles on one target; each
# is its own binding, and the scope roles contain each other: viewer < editor < owner.
resource "fpcloud_iam_binding" "jane_editor" {
  project_id = fpcloud_project.production.id
  member     = "user:jane@example.com"
  role       = "editor"
}

# A billing role is a binding on the organization. An owner holds
# billing.admin by inclusion; this gives someone the bill and nothing else.
resource "fpcloud_iam_binding" "finance_reads_the_bill" {
  org_id = fpcloud_project.production.organization_id
  member = "user:finance@example.com"
  role   = "billing.viewer"
}

# A resource handed to an app's own identity, with no secret reference in the
# app's env: the binding alone is the grant. The role is the kind's handed
# role: `database.user`, `bucket.user` or `secret.reader`.
resource "fpcloud_iam_binding" "api_uses_orders" {
  project_id = fpcloud_project.production.id
  resource   = "database/${fpcloud_database.orders.id}"
  member     = "serviceAccount:${fpcloud_app.api.service_account}"
  role       = "database.user"
}
