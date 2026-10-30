data "rintup_locations" "all" {}

# Allow-list RintUp probes in a firewall.
output "rintup_probe_ipv4" {
  value = flatten(data.rintup_locations.all.locations[*].ipv4)
}

# Check from every location.
resource "rintup_monitor" "everywhere" {
  type      = "http"
  locations = data.rintup_locations.all.codes
  http      = { url = "https://www.example.com" }
}
