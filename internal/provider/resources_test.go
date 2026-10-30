// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func channelConfig(f *fakeAPI, secretVersion string) string {
	return f.providerConfig() + `
resource "rintup_contact_group" "ops" {
  name = "Ops"
}
resource "rintup_alert_channel" "hook" {
  contact_group_id  = rintup_contact_group.ops.id
  type              = "webhook"
  target            = "https://hooks.example.com/rintup"
  secret_wo         = "s3cr3t-signing-` + secretVersion + `"
  secret_wo_version = "` + secretVersion + `"
}
resource "rintup_alert_channel" "slack" {
  contact_group_id  = rintup_contact_group.ops.id
  type              = "slack"
  target_wo         = "https://hooks.slack.com/services/T0/B0/s3cr3t-slack"
  target_wo_version = "1"
  locale            = "tr"
}
resource "rintup_alert_channel" "generated" {
  contact_group_id = rintup_contact_group.ops.id
  type             = "webhook"
  target           = "https://hooks.example.com/other"
}
`
}

func TestContactGroupAndAlertChannels(t *testing.T) {
	f := newFakeAPI(t)
	var hookID string
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				Config: channelConfig(f, "1"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("rintup_contact_group.ops", "name", "Ops"),
					resource.TestCheckResourceAttr("rintup_alert_channel.slack", "locale", "tr"),
					resource.TestCheckResourceAttr("rintup_alert_channel.generated", "generated_secret", "whsec_generated"),
					resource.TestCheckNoResourceAttr("rintup_alert_channel.hook", "generated_secret"),
					resource.TestCheckResourceAttrWith("rintup_alert_channel.hook", "id", func(v string) error { hookID = v; return nil }),
					noSecretInState("s3cr3t"),
					check(func() error {
						if f.channelSec[hookID] != "s3cr3t-signing-1" {
							return fmt.Errorf("webhook secret not delivered")
						}
						for _, v := range f.channelSec {
							if v == "https://hooks.slack.com/services/T0/B0/s3cr3t-slack" {
								return nil
							}
						}
						return fmt.Errorf("slack target_wo not delivered")
					}),
				),
			},
			{
				// The API cannot update a channel: a new secret version recreates it.
				Config: channelConfig(f, "2"),
				Check: resource.ComposeTestCheckFunc(
					noSecretInState("s3cr3t"),
					resource.TestCheckResourceAttrWith("rintup_alert_channel.hook", "id", func(v string) error {
						if v == hookID || f.channelSec[v] != "s3cr3t-signing-2" {
							return fmt.Errorf("channel not replaced with the new secret")
						}
						return nil
					}),
				),
			},
			{
				// Group renamed in the panel: drift.
				PreConfig: func() {
					for _, g := range f.groups {
						g.Name = "Renamed"
					}
				},
				Config:             channelConfig(f, "2"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{Config: channelConfig(f, "2")},
			{ResourceName: "rintup_contact_group.ops", ImportState: true, ImportStateVerify: true},
			{
				// Import by "<group>/<channel>"; target, secret versions and locale are not returned by the API.
				ResourceName: "rintup_alert_channel.slack",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["rintup_alert_channel.slack"]
					return rs.Primary.Attributes["contact_group_id"] + "/" + rs.Primary.ID, nil
				},
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"target_wo_version", "locale"},
			},
			{
				// Import by channel id alone: every group is searched.
				ResourceName:            "rintup_alert_channel.hook",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"target", "secret_wo_version", "locale"},
			},
		},
	})
}

func TestMaintenanceWindows(t *testing.T) {
	f := newFakeAPI(t)
	cfg := func(endsAt string) string {
		return f.providerConfig() + `
resource "rintup_maintenance_window" "deploy" {
  count       = 3 # more windows than one page of the fake API: Read must follow nextCursor
  name        = "Deploy ${count.index}"
  monitor_ids = []
}
resource "rintup_maintenance_window" "weekly" {
  monitor_ids = []
  recurrence  = "weekly"
  days        = [0, 6]
  start_time  = "03:00"
  starts_at   = "2026-11-01T00:00:00+00:00"
` + endsAt + `
}
`
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				Config: cfg(`  ends_at = "2027-01-01T00:00:00Z"`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("rintup_maintenance_window.deploy.2", "name", "Deploy 2"),
					resource.TestCheckResourceAttr("rintup_maintenance_window.deploy.0", "recurrence", "none"),
					resource.TestCheckResourceAttrSet("rintup_maintenance_window.deploy.0", "ends_at"),
					resource.TestCheckResourceAttr("rintup_maintenance_window.weekly", "duration_minutes", "60"),
					resource.TestCheckResourceAttr("rintup_maintenance_window.weekly", "days.#", "2"),
				),
			},
			{
				// Removing ends_at from a recurring window sends "endsAt": null (a series without end).
				Config: cfg(""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr("rintup_maintenance_window.weekly", "ends_at"),
					check(func() error {
						for _, w := range f.windows {
							if w.Recurrence == "weekly" && w.EndsAt != nil {
								return fmt.Errorf("endsAt not cleared on the server")
							}
						}
						return nil
					}),
				),
			},
			{ResourceName: "rintup_maintenance_window.weekly", ImportState: true, ImportStateVerify: true},
		},
	})
}

func statusPageConfig(f *fakeAPI, password string) string {
	return f.providerConfig() + `
resource "rintup_monitor" "api" {
  type = "http"
  http = { url = "https://api.example.com/health" }
}
resource "rintup_status_page" "acme" {
  title       = "Acme Status"
  description = "Live status"
  components = [
    { monitor_id = rintup_monitor.api.id },
    { monitor_id = rintup_monitor.api.id, display_name = "API (EU)", group = "Core" },
  ]
` + password + `
}
`
}

func TestStatusPage(t *testing.T) {
	f := newFakeAPI(t)
	pageID := func() string {
		for id := range f.pages {
			return id
		}
		return ""
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				Config: statusPageConfig(f, `  password_wo = "s3cr3t-page"`+"\n"+`  password_wo_version = "1"`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("rintup_status_page.acme", "slug", "acme-status"),
					resource.TestCheckResourceAttr("rintup_status_page.acme", "components.0.display_name", "api.example.com"),
					resource.TestCheckResourceAttr("rintup_status_page.acme", "components.1.group", "Core"),
					resource.TestCheckResourceAttr("rintup_status_page.acme", "password_protected", "true"),
					noSecretInState("s3cr3t"),
					check(func() error {
						if f.passwords[pageID()] != "s3cr3t-page" {
							return fmt.Errorf("password not delivered")
						}
						return nil
					}),
				),
			},
			{
				// Removing the password together with its version removes the protection.
				Config: statusPageConfig(f, ""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("rintup_status_page.acme", "password_protected", "false"),
					check(func() error {
						if _, ok := f.passwords[pageID()]; ok {
							return fmt.Errorf("password still set")
						}
						return nil
					}),
				),
			},
			{
				PreConfig:          func() { f.pages[pageID()].Description = nil },
				Config:             statusPageConfig(f, ""),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{Config: statusPageConfig(f, "")},
			{ResourceName: "rintup_status_page.acme", ImportState: true, ImportStateVerify: true},
		},
	})
}

func TestDataSources(t *testing.T) {
	f := newFakeAPI(t)
	base := f.providerConfig() + `
resource "rintup_monitor" "m" {
  count = 3
  type  = "tcp"
  name  = "svc-${count.index}"
  tcp   = { host = "svc${count.index}.example.com", port = 443 }
}
`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{Config: base},
			{
				Config: base + `
data "rintup_locations" "all" {}
data "rintup_monitor" "by_name" {
  name = "svc-2" # on the second page of the fake API
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.rintup_locations.all", "codes.#", "2"),
					resource.TestCheckResourceAttr("data.rintup_locations.all", "locations.0.ipv4.0", "192.0.2.10"),
					resource.TestCheckResourceAttrPair("data.rintup_monitor.by_name", "id", "rintup_monitor.m.2", "id"),
					resource.TestCheckResourceAttr("data.rintup_monitor.by_name", "target", "svc2.example.com:443"),
				),
			},
			{
				Config:      base + `data "rintup_monitor" "missing" { name = "nope" }`,
				ExpectError: regexp.MustCompile(`0 monitors are named "nope"`),
			},
		},
	})
}
