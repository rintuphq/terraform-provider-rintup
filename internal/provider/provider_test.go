// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestProviderNeedsAPIKey(t *testing.T) {
	t.Setenv("RINTUP_API_KEY", "")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{{
			Config:      `data "rintup_locations" "all" {}`,
			ExpectError: regexp.MustCompile(`Missing RintUp API key`),
		}},
	})
}

func TestProviderReadsEnvironment(t *testing.T) {
	f := newFakeAPI(t)
	t.Setenv("RINTUP_API_KEY", "rup_live_test")
	t.Setenv("RINTUP_BASE_URL", f.srv.URL+"/api/v1")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{{
			Config: `
provider "rintup" {}
data "rintup_locations" "all" {}
`,
			Check: resource.TestCheckResourceAttr("data.rintup_locations.all", "locations.1.code", "ist"),
		}},
	})
}
