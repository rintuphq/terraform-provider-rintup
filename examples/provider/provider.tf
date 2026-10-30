terraform {
  required_version = ">= 1.11" # write-only secrets (*_wo) need Terraform 1.11+
  required_providers {
    rintup = {
      source = "rintuphq/rintup"
    }
  }
}

# The API key comes from RINTUP_API_KEY when api_key is not set.
provider "rintup" {
  # api_key  = var.rintup_api_key                # rup_live_...; prefer the environment variable
  # base_url = "https://rintup.com/api/v1"       # or RINTUP_BASE_URL
}
