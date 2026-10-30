# A monitor created in the panel (or by another team's configuration), found by its exact name.
data "rintup_monitor" "billing" {
  name = "billing.example.com"
}

resource "rintup_status_page" "billing" {
  title      = "Billing Status"
  components = [{ monitor_id = data.rintup_monitor.billing.id }]
}
