// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// These tests run the real Terraform CLI (>= 1.11 for write-only attributes) against fakeAPI.
// They need no network and no TF_ACC: resource.UnitTest always runs.

var testProviders = map[string]func() (tfprotov6.ProviderServer, error){
	"rintup": providerserver.NewProtocol6WithError(New("test")()),
}

// noSecretInState fails when any attribute of any resource contains one of the secrets.
func noSecretInState(secrets ...string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for name, rs := range s.RootModule().Resources {
			for k, v := range rs.Primary.Attributes {
				for _, secret := range secrets {
					if strings.Contains(v, secret) {
						return fmt.Errorf("%s.%s stores a secret in state", name, k)
					}
				}
			}
		}
		return nil
	}
}

func check(fn func() error) resource.TestCheckFunc {
	return func(*terraform.State) error { return fn() }
}

func monitorConfig(f *fakeAPI, extra, httpExtra string) string {
	return f.providerConfig() + `
resource "rintup_monitor" "api" {
  type = "http"
  tags = ["prod"]
` + extra + `
  http = {
    url                            = "https://api.example.com/health"
    headers_wo                     = { Authorization = "Bearer s3cr3t-hdr-1" }
    basic_auth_user                = "ops"
    basic_auth_password_wo         = "s3cr3t-pass"
    basic_auth_password_wo_version = "1"
` + httpExtra + `
  }
}
`
}

func TestMonitorHTTPLifecycle(t *testing.T) {
	f := newFakeAPI(t)
	var firstID string
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				// Create with server defaults (name, interval, locations...) read back: the automatic
				// post-apply plan must be empty. The first requests are throttled (429 + Retry-After).
				PreConfig: func() { f.throttle = 3 },
				Config:    monitorConfig(f, "", `    headers_wo_version = "1"`+"\n"+`    keyword_present = "ok"`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("rintup_monitor.api", "name", "api.example.com"),
					resource.TestCheckResourceAttr("rintup_monitor.api", "interval_seconds", "300"),
					resource.TestCheckResourceAttr("rintup_monitor.api", "locations.#", "2"),
					resource.TestCheckResourceAttr("rintup_monitor.api", "http.method", "GET"),
					resource.TestCheckResourceAttr("rintup_monitor.api", "http.keyword_present", "ok"),
					resource.TestCheckResourceAttr("rintup_monitor.api", "http.headers_wo_version", "1"),
					resource.TestCheckNoResourceAttr("rintup_monitor.api", "http.headers_wo"),
					resource.TestCheckNoResourceAttr("rintup_monitor.api", "http.basic_auth_password_wo"),
					resource.TestCheckResourceAttrWith("rintup_monitor.api", "id", func(v string) error { firstID = v; return nil }),
					noSecretInState("s3cr3t"),
					check(func() error {
						if f.throttled != 3 {
							return fmt.Errorf("throttled = %d, want 3 retried requests", f.throttled)
						}
						sec := f.secrets[firstID]
						if sec.Headers["Authorization"] != "Bearer s3cr3t-hdr-1" || sec.Password != "s3cr3t-pass" {
							return fmt.Errorf("secrets not delivered: %+v", sec)
						}
						return nil
					}),
				),
			},
			{
				// Update: keyword removed (cleared with ""), paused, new header sent because its version moved;
				// the password version did not move, so the stored password is untouched.
				Config: strings.Replace(monitorConfig(f, `  name = "Public API"`+"\n"+`  paused = true`, `    headers_wo_version = "2"`),
					"s3cr3t-hdr-1", "s3cr3t-hdr-2", 1),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("rintup_monitor.api", "name", "Public API"),
					resource.TestCheckResourceAttr("rintup_monitor.api", "paused", "true"),
					resource.TestCheckNoResourceAttr("rintup_monitor.api", "http.keyword_present"),
					noSecretInState("s3cr3t"),
					check(func() error {
						m, sec := f.monitors[firstID], f.secrets[firstID]
						if m.HTTP.KeywordPresent != nil || !m.Paused {
							return fmt.Errorf("server not updated: %+v", m)
						}
						if sec.Headers["Authorization"] != "Bearer s3cr3t-hdr-2" || sec.Password != "s3cr3t-pass" {
							return fmt.Errorf("secrets = %+v", sec)
						}
						return nil
					}),
				),
			},
			{
				// Drift: a configured field changed in the panel shows up in the plan. (Fields left to the
				// server default, like interval_seconds here, adopt the panel value instead.)
				PreConfig: func() { f.monitors[firstID].Name = "renamed in panel"; f.monitors[firstID].IntervalSeconds = 60 },
				Config: strings.Replace(monitorConfig(f, `  name = "Public API"`+"\n"+`  paused = true`, `    headers_wo_version = "2"`),
					"s3cr3t-hdr-1", "s3cr3t-hdr-2", 1),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				// Deleted in the panel (404): removed from state and planned for creation again.
				PreConfig: func() { delete(f.monitors, firstID) },
				Config: strings.Replace(monitorConfig(f, `  name = "Public API"`+"\n"+`  paused = true`, `    headers_wo_version = "2"`),
					"s3cr3t-hdr-1", "s3cr3t-hdr-2", 1),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: strings.Replace(monitorConfig(f, `  name = "Public API"`+"\n"+`  paused = true`, `    headers_wo_version = "2"`),
					"s3cr3t-hdr-1", "s3cr3t-hdr-2", 1),
				Check: resource.TestCheckResourceAttrWith("rintup_monitor.api", "id", func(v string) error {
					if v == firstID {
						return fmt.Errorf("expected a new monitor after the 404")
					}
					return nil
				}),
			},
			{
				ResourceName:      "rintup_monitor.api",
				ImportState:       true,
				ImportStateVerify: true,
				// The API never returns secrets, so their versions cannot be imported.
				ImportStateVerifyIgnore: []string{"http.headers_wo_version", "http.basic_auth_password_wo_version"},
			},
		},
	})
}

func TestMonitorPushAndTCP(t *testing.T) {
	f := newFakeAPI(t)
	cfg := func(send string) string {
		return f.providerConfig() + `
resource "rintup_monitor" "job" {
  type = "push"
}
resource "rintup_monitor" "db" {
  type = "tcp"
  tcp = {
    host = "db.example.com"
    port = 5432
` + send + `
  }
}
`
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				Config: cfg(`    send = "PING"`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("rintup_monitor.job", "push.grace_seconds", "60"),
					resource.TestCheckResourceAttr("rintup_monitor.job", "push.period_seconds", "300"),
					resource.TestCheckResourceAttrSet("rintup_monitor.job", "push_url"),
					resource.TestCheckResourceAttr("rintup_monitor.db", "name", "db.example.com:5432"),
					resource.TestCheckResourceAttr("rintup_monitor.db", "tcp.send", "PING"),
				),
			},
			{
				Config: cfg(""),
				Check:  resource.TestCheckNoResourceAttr("rintup_monitor.db", "tcp.send"),
			},
		},
	})
}

func TestMonitorBlockMustMatchType(t *testing.T) {
	f := newFakeAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				Config:      f.providerConfig() + `resource "rintup_monitor" "x" { type = "http" }`,
				ExpectError: regexp.MustCompile(`Missing http block`),
			},
			{
				Config: f.providerConfig() + `resource "rintup_monitor" "x" {
  type = "ping"
  ping = { host = "example.com" }
  tcp  = { host = "example.com", port = 22 }
}`,
				ExpectError: regexp.MustCompile(`Block does not match type`),
			},
		},
	})
}
