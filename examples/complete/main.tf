# Complete example: the health of an API, its TLS certificate, a Slack channel for alerts and a
# public status page.
#
#   export RINTUP_API_KEY=rup_live_...
#   export TF_VAR_slack_webhook_url=https://hooks.slack.com/services/...
#   terraform init && terraform apply
#
# A second `terraform plan` must report "No changes": server defaults are read back into state.

terraform {
  required_version = ">= 1.11"
  required_providers {
    rintup = {
      source = "rintuphq/rintup"
    }
  }
}

provider "rintup" {}

variable "api_host" {
  type    = string
  default = "api.example.com"
}

variable "slack_webhook_url" {
  description = "Slack incoming webhook URL (a credential: it is sent write-only, never stored in state)."
  type        = string
  sensitive   = true
}

variable "slack_webhook_version" {
  description = "Bump after rotating the Slack webhook; the channel is then recreated with the new URL."
  type        = string
  default     = "1"
}

resource "rintup_contact_group" "api_team" {
  name = "API team"
}

resource "rintup_alert_channel" "slack" {
  contact_group_id  = rintup_contact_group.api_team.id
  type              = "slack"
  target_wo         = var.slack_webhook_url
  target_wo_version = var.slack_webhook_version
}

resource "rintup_monitor" "api_health" {
  type              = "http"
  name              = "API health"
  interval_seconds  = 60
  contact_group_ids = [rintup_contact_group.api_team.id]
  tags              = ["api", "prod"]

  http = {
    url             = "https://${var.api_host}/health"
    keyword_present = "ok"
  }
}

resource "rintup_monitor" "api_certificate" {
  type              = "ssl"
  name              = "API certificate"
  contact_group_ids = [rintup_contact_group.api_team.id]

  ssl = {
    host            = var.api_host
    thresholds_days = [30, 14, 7]
  }
}

resource "rintup_status_page" "public" {
  title       = "Example API Status"
  description = "Availability of the Example public API"
  locale      = "en"

  allow_subscriptions          = true
  notify_subscribers_on_outage = true

  components = [
    { monitor_id = rintup_monitor.api_health.id, display_name = "API", group = "Public API" },
    { monitor_id = rintup_monitor.api_certificate.id, display_name = "TLS certificate", group = "Public API" },
  ]
}

output "status_page_url" {
  value = "https://status.rintup.com/${rintup_status_page.public.slug}"
}
