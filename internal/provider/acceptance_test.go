// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Acceptance tests run against a real RintUp workspace and create (then destroy) real objects:
//
//	TF_ACC=1 RINTUP_API_KEY=rup_live_... RINTUP_BASE_URL=https://rintup.com/api/v1 go test ./internal/provider -run TestAcc -v
//
// Use a dedicated test workspace. Without TF_ACC=1 they are skipped.

func testAccPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("RINTUP_API_KEY") == "" {
		t.Fatal("RINTUP_API_KEY must be set for acceptance tests")
	}
}

func testAccConfig(suffix, extra string) string {
	return `
provider "rintup" {} # RINTUP_API_KEY and RINTUP_BASE_URL from the environment

resource "rintup_contact_group" "ops" {
  name = "tf-acc-` + suffix + `"
}

resource "rintup_alert_channel" "hook" {
  contact_group_id  = rintup_contact_group.ops.id
  type              = "webhook"
  target            = "https://example.com/tf-acc-hook"
  secret_wo         = "tf-acc-signing-secret"
  secret_wo_version = "1"
}

resource "rintup_monitor" "api" {
  type              = "http"
  name              = "tf-acc-` + suffix + `"
  contact_group_ids = [rintup_contact_group.ops.id]
  tags              = ["tf-acc"]
` + extra + `
  http = {
    url                = "https://example.com/"
    headers_wo         = { "X-Tf-Acc" = "tf-acc-header-secret" }
    headers_wo_version = "1"
  }
}

resource "rintup_monitor" "cert" {
  type = "ssl"
  name = "tf-acc-ssl-` + suffix + `"
  ssl  = { host = "example.com" }
}

resource "rintup_maintenance_window" "nightly" {
  name        = "tf-acc-` + suffix + `"
  monitor_ids = [rintup_monitor.api.id]
  recurrence  = "daily"
  start_time  = "03:00"
}

resource "rintup_status_page" "page" {
  title      = "tf-acc ` + suffix + `"
  components = [{ monitor_id = rintup_monitor.api.id }, { monitor_id = rintup_monitor.cert.id, group = "Certificates" }]
}

data "rintup_locations" "all" {}

data "rintup_monitor" "by_name" {
  name       = "tf-acc-` + suffix + `"
  depends_on = [rintup_monitor.api]
}
`
}

func TestAccFullStack(t *testing.T) {
	suffix := acctest.RandString(8)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				// apply, then the framework's automatic plan must be empty (no drift from server defaults)
				Config: testAccConfig(suffix, ""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("rintup_monitor.api", "interval_seconds"),
					resource.TestCheckResourceAttrPair("data.rintup_monitor.by_name", "id", "rintup_monitor.api", "id"),
					resource.TestCheckResourceAttrSet("data.rintup_locations.all", "codes.0"),
					noSecretInState("tf-acc-signing-secret", "tf-acc-header-secret"),
				),
			},
			{
				Config: testAccConfig(suffix, `  paused = true`),
				Check:  resource.TestCheckResourceAttr("rintup_monitor.api", "paused", "true"),
			},
			{
				ResourceName:            "rintup_monitor.api",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"http.headers_wo_version", "status"},
			},
			{ResourceName: "rintup_contact_group.ops", ImportState: true, ImportStateVerify: true},
			{ResourceName: "rintup_maintenance_window.nightly", ImportState: true, ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"active", "next_starts_at", "next_ends_at"}},
			{ResourceName: "rintup_status_page.page", ImportState: true, ImportStateVerify: true},
			{
				ResourceName: "rintup_alert_channel.hook",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["rintup_alert_channel.hook"]
					return rs.Primary.Attributes["contact_group_id"] + "/" + rs.Primary.ID, nil
				},
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"target", "secret_wo_version", "locale", "verified"},
			},
		},
	})
}
