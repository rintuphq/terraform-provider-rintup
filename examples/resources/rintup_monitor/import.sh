# Import by monitor id (Settings > Monitors, or GET /api/v1/monitors).
# Secrets are never returned by the API, so the *_wo_version values cannot be imported:
# the first apply after an import sends the configured secrets once.
terraform import rintup_monitor.api 0192f3a4-5b6c-7d8e-9f00-112233445566
