# An organization is granted by an operator together with its first owner, so
# there is nothing here to declare — only to read. Reference it by its opaque
# id, which is frozen; the readable name can be changed at any time.
data "fpcloud_org" "acme" {
  id = "acm"
}

# The ceiling every project in the org shares. Read-only: only an operator moves
# it, and it bounds the org rather than a project because you decide how many
# projects you have.
output "org_cpu_ceiling" {
  value = data.fpcloud_org.acme.max_cpu
}
