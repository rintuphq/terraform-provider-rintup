# "<contact_group_id>/<channel_id>" (fast) or just "<channel_id>" (all groups are searched).
# The API never returns a channel's target, secret or locale: after an import the first apply
# adopts them from the configuration without recreating the channel.
terraform import rintup_alert_channel.slack 0192f3a4-0000-7000-8000-000000000001/0192f3a4-0000-7000-8000-000000000002
