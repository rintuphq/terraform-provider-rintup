# The smallest monitor: only the type and its target. Everything else (name, interval,
# locations, confirmations...) takes the server default and is read back into state.
resource "rintup_monitor" "home" {
  type = "http"
  http = { url = "https://www.example.com" }
}

# An HTTP check with secrets. headers_wo / basic_auth_password_wo are write-only: Terraform sends
# them but never stores them in state. Bump the *_version value to send a new secret.
variable "api_token" {
  type      = string
  sensitive = true
}

resource "rintup_monitor" "api" {
  type              = "http"
  name              = "Public API"
  interval_seconds  = 60
  locations         = ["fra", "ist"]
  contact_group_ids = [rintup_contact_group.ops.id]
  tags              = ["prod", "api"]

  http = {
    url             = "https://api.example.com/health"
    method          = "GET"
    keyword_present = "\"status\":\"ok\""
    expected_status = [{ from = 200, to = 299 }]

    headers_wo         = { Authorization = "Bearer ${var.api_token}" }
    headers_wo_version = "2026-10-30" # change when the token rotates
  }
}

resource "rintup_monitor" "db" {
  type = "tcp"
  tcp  = { host = "db.example.com", port = 5432 }
}

resource "rintup_monitor" "dns" {
  type = "dns"
  dns  = { host = "example.com", record_type = "MX", expected = ["mail.example.com"] }
}

resource "rintup_monitor" "ping" {
  type = "ping"
  ping = { host = "203.0.113.10", count = 3 }
}

resource "rintup_monitor" "cert" {
  type = "ssl"
  ssl  = { host = "example.com", thresholds_days = [30, 14, 7] }
}

resource "rintup_monitor" "domain" {
  type   = "domain"
  domain = { domain = "example.com", expected_nameservers = ["ns1.example.net", "ns2.example.net"] }
}

resource "rintup_monitor" "speed" {
  type      = "pagespeed"
  pagespeed = { url = "https://www.example.com", max_total_ms = 3000 }
}

# Heartbeat: your cron job calls push_url; no block needed.
resource "rintup_monitor" "backup_job" {
  type             = "push"
  interval_seconds = 86400
  push             = { grace_seconds = 600 }
}

output "backup_push_url" {
  value     = rintup_monitor.backup_job.push_url
  sensitive = true
}
