# One-off window for a release.
resource "rintup_maintenance_window" "release" {
  name        = "Release 4.2"
  monitor_ids = [rintup_monitor.api.id]
  starts_at   = "2026-11-15T22:00:00Z"
  ends_at     = "2026-11-15T23:30:00Z"
}

# Every Sunday 03:00-04:00 Istanbul time, for all monitors (empty set).
resource "rintup_maintenance_window" "weekly_backup" {
  name             = "Weekly backup"
  monitor_ids      = []
  recurrence       = "weekly"
  days             = [0]
  start_time       = "03:00"
  duration_minutes = 60
  time_zone        = "Europe/Istanbul"
}
