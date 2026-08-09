# A spend target for the organization. It alerts; it never caps — crossing a
# threshold changes nothing about what runs.
resource "fpcloud_billing_budget" "monthly" {
  org_id = var.org_id
  amount = "250.00"

  # Percentages of `amount`. Above 100 is allowed: 150 is how you hear about it
  # again once you are well past the target.
  thresholds = [50, 90, 100, 150]
}
