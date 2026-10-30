resource "rintup_status_page" "public" {
  title       = "Acme Status"
  slug        = "acme"
  description = "Live status of Acme services"
  brand_color = "#0F766E"
  locale      = "en"

  allow_subscriptions          = true
  notify_subscribers_on_outage = true

  # Display order = list order. display_name defaults to the monitor's name.
  components = [
    { monitor_id = rintup_monitor.api.id, display_name = "API", group = "Core" },
    { monitor_id = rintup_monitor.home.id, display_name = "Website" },
  ]
}

# A private page: the password is write-only (never stored in state).
variable "status_password" {
  type      = string
  sensitive = true
}

resource "rintup_status_page" "internal" {
  title               = "Acme Internal"
  noindex             = true
  password_wo         = var.status_password
  password_wo_version = "1" # bump to set a new password; remove both to drop the protection
}
