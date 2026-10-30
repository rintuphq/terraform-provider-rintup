# rintup_contact_group.ops: see the rintup_contact_group example.

# E-mail: a plain target. The address receives a verification link first.
resource "rintup_alert_channel" "email" {
  contact_group_id = rintup_contact_group.ops.id
  type             = "email"
  target           = "ops@example.com"
  locale           = "tr"
}

# Slack: the incoming webhook URL is a credential, so it goes to the write-only target_wo.
variable "slack_webhook_url" {
  type      = string
  sensitive = true
}

resource "rintup_alert_channel" "slack" {
  contact_group_id  = rintup_contact_group.ops.id
  type              = "slack"
  target_wo         = var.slack_webhook_url
  target_wo_version = "1" # bump after rotating the webhook: the channel is recreated
}

# Webhook with your own signing key (write-only). Without secret_wo the server generates one and
# returns it once; the provider then keeps it in the sensitive generated_secret attribute.
variable "webhook_signing_key" {
  type      = string
  sensitive = true
}

resource "rintup_alert_channel" "webhook" {
  contact_group_id  = rintup_contact_group.ops.id
  type              = "webhook"
  target            = "https://hooks.example.com/rintup"
  secret_wo         = var.webhook_signing_key
  secret_wo_version = "1"
}

# PagerDuty: the Events API v2 routing key is the secret.
variable "pagerduty_routing_key" {
  type      = string
  sensitive = true
}

resource "rintup_alert_channel" "pagerduty" {
  contact_group_id  = rintup_contact_group.ops.id
  type              = "pagerduty"
  secret_wo         = var.pagerduty_routing_key
  secret_wo_version = "1"
}
