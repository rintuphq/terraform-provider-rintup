// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var monitorTypes = []string{"http", "tcp", "dns", "ping", "ssl", "domain", "pagespeed", "push"}

var allowedIntervals = []int64{30, 60, 120, 300, 600, 900, 1800, 3600, 21600, 43200, 86400}

var statusRangeType = types.ObjectType{AttrTypes: map[string]attr.Type{"from": types.Int64Type, "to": types.Int64Type}}

// Helpers that keep the schema readable. "computed" attributes take the server value when not
// configured and keep it between plans (UseStateForUnknown), so server defaults cause no plan noise.

func optString(desc string) schema.StringAttribute {
	return schema.StringAttribute{Optional: true, Description: desc}
}

func computedString(desc string) schema.StringAttribute {
	return schema.StringAttribute{Optional: true, Computed: true, Description: desc,
		PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}
}

func defaultString(desc, def string, allowed ...string) schema.StringAttribute {
	a := schema.StringAttribute{Optional: true, Computed: true, Description: desc + " Default: `" + def + "`.",
		Default: stringdefault.StaticString(def)}
	if len(allowed) > 0 {
		a.Validators = []validator.String{stringvalidator.OneOf(allowed...)}
	}
	return a
}

func defaultInt(desc string, def int64) schema.Int64Attribute {
	return schema.Int64Attribute{Optional: true, Computed: true, Description: desc,
		Default: int64default.StaticInt64(def)}
}

func computedInt(desc string) schema.Int64Attribute {
	return schema.Int64Attribute{Optional: true, Computed: true, Description: desc,
		PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}}
}

func emptySet(desc string, elem attr.Type) schema.SetAttribute {
	return schema.SetAttribute{Optional: true, Computed: true, ElementType: elem, Description: desc,
		Default: setdefault.StaticValue(types.SetValueMust(elem, []attr.Value{}))}
}

func computedSet(desc string, elem attr.Type) schema.SetAttribute {
	return schema.SetAttribute{Optional: true, Computed: true, ElementType: elem, Description: desc,
		PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()}}
}

// secretPair is a write-only secret plus its "_version" companion. The value is sent to the API
// but never stored in plan or state (Terraform >= 1.11); bump the version to send a new value.
func secretPair(attrs map[string]schema.Attribute, name, what string) {
	attrs[name] = schema.StringAttribute{Optional: true, Sensitive: true, WriteOnly: true,
		Description: what + " Write-only: never stored in state. Change `" + name + "_version` to send a new value."}
	attrs[name+"_version"] = schema.StringAttribute{Optional: true,
		Description: "Any value (e.g. `1` or a date). When it changes, `" + name + "` is sent again; " +
			"when it is removed together with the secret, the stored secret is deleted."}
}

func typeBlock(desc string, attrs map[string]schema.Attribute) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{Optional: true, Description: desc, Attributes: attrs}
}

func monitorSchema() schema.Schema {
	return schema.Schema{
		Description: "An uptime monitor. Only `type` and the target in the block of that type are required; " +
			"everything else has a server default that is read back into state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, Description: "Monitor id (UUID).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"type": schema.StringAttribute{Required: true,
				Description:   "One of `http`, `tcp`, `dns`, `ping`, `ssl`, `domain`, `pagespeed`, `push`. Changing it replaces the monitor.",
				Validators:    []validator.String{stringvalidator.OneOf(monitorTypes...)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"name": computedString("Display name. Default: derived from the target (host or URL)."),
			"interval_seconds": schema.Int64Attribute{Optional: true, Computed: true,
				Description:   "Check interval in seconds: 30, 60, 120, 300, 600, 900, 1800, 3600, 21600, 43200 or 86400. Default: 300 or the plan minimum (ssl >= 3600, domain >= 21600).",
				Validators:    []validator.Int64{int64validator.OneOf(allowedIntervals...)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"timeout_ms":            computedInt("Timeout in milliseconds. Default: 10000 (domain 30000, pagespeed 60000)."),
			"locations":             computedSet("Probe location codes (see the `rintup_locations` data source). Default: the first online locations allowed by the plan.", types.StringType),
			"confirmations":         defaultInt("Consecutive failed checks per location before it counts as down. Default: `2`.", 2),
			"aggregate_rule":        defaultString("How locations are combined: `majority`, `any` or `all`.", "majority", "majority", "any", "all"),
			"trigger_delay_seconds": defaultInt("Wait this long while down before alerting. Default: `0`.", 0),
			"repeat_every_minutes":  defaultInt("Repeat the alert every N minutes while down; 0 = once. Default: `0`.", 0),
			"contact_group_ids":     emptySet("Contact groups (`rintup_contact_group`) that receive alerts.", types.StringType),
			"tags":                  emptySet("Free-form tags.", types.StringType),
			"paused": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "Pause checks (POST /monitors/{id}/pause and /resume). Default: `false`."},
			"target":     schema.StringAttribute{Computed: true, Description: "What is checked, as shown in the panel."},
			"status":     schema.StringAttribute{Computed: true, Description: "Current status (`up`, `down`, `pending`...). Informational; it does not cause a diff."},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"push_url": schema.StringAttribute{Computed: true, Sensitive: true,
				Description:   "Push (heartbeat) monitors: the path your job calls, relative to the API host. It contains the push token.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"http":      typeBlock("HTTP(S) check; required when `type = \"http\"`.", httpAttributes()),
			"tcp":       typeBlock("TCP port check; required when `type = \"tcp\"`.", tcpAttributes()),
			"dns":       typeBlock("DNS record check; required when `type = \"dns\"`.", dnsAttributes()),
			"ping":      typeBlock("ICMP ping; required when `type = \"ping\"`.", pingAttributes()),
			"ssl":       typeBlock("TLS certificate expiry; required when `type = \"ssl\"`.", sslAttributes()),
			"domain":    typeBlock("Domain registration expiry and name servers; required when `type = \"domain\"`.", domainAttributes()),
			"pagespeed": typeBlock("Page load time; required when `type = \"pagespeed\"`.", pageSpeedAttributes()),
			"push": schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: pushAttributes(),
				Description:   "Heartbeat settings for `type = \"push\"` (optional; the server fills defaults).",
				PlanModifiers: []planmodifier.Object{objectplanmodifier.UseStateForUnknown()}},
		},
	}
}

func httpAttributes() map[string]schema.Attribute {
	attrs := map[string]schema.Attribute{
		"url":             schema.StringAttribute{Required: true, Description: "http(s) address to check. When its scheme, host or port changes, stored secrets are not carried over: change `headers_wo_version`, `body_wo_version` and `basic_auth_password_wo_version` (for the ones set) in the same apply, or the update fails with RUP-MON-400-SECRETS_REENTRY_REQUIRED."},
		"method":          defaultString("`GET`, `HEAD`, `POST`, `PUT`, `PATCH` or `DELETE`.", "GET", "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE"),
		"content_type":    optString("Content-Type of the request body."),
		"basic_auth_user": optString("Basic authentication user name."),
		"keyword_present": optString("The response must contain this text."),
		"keyword_absent":  optString("The response must not contain this text."),
		"follow_redirects": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true),
			Description: "Follow redirects. Default: `true`."},
		"user_agent": optString("User-Agent header of the check."),
		"expected_status": schema.ListNestedAttribute{Optional: true, Computed: true,
			Description: "Accepted status ranges. Default: none (any 2xx).",
			Default:     listdefault.StaticValue(types.ListValueMust(statusRangeType, []attr.Value{})),
			NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"from": schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.Between(100, 599)}},
				"to":   schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.Between(100, 599)}},
			}}},
		"headers_wo": schema.MapAttribute{Optional: true, Sensitive: true, WriteOnly: true, ElementType: types.StringType,
			Description: "Request headers (all values are treated as secrets). Write-only: never stored in state. " +
				"Change `headers_wo_version` to send the map again (it replaces all headers)."},
		"headers_wo_version": schema.StringAttribute{Optional: true,
			Description: "Any value; when it changes `headers_wo` is sent again."},
	}
	secretPair(attrs, "body_wo", "Request body.")
	secretPair(attrs, "basic_auth_password_wo", "Basic authentication password.")
	return attrs
}

func tcpAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"host":   schema.StringAttribute{Required: true, Description: "Host name or IP."},
		"port":   schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.Between(1, 65535)}},
		"send":   optString("Text to send after connecting."),
		"expect": optString("Text the answer must contain."),
	}
}

func dnsAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"host":        schema.StringAttribute{Required: true, Description: "Name to resolve."},
		"record_type": defaultString("`A`, `AAAA`, `CNAME`, `MX`, `TXT` or `NS`.", "A", "A", "AAAA", "CNAME", "MX", "TXT", "NS"),
		"expected":    emptySet("Expected answers (any order). Empty: any answer.", types.StringType),
		"resolver":    optString("Resolver IP. Default: the probe's resolver."),
	}
}

func pingAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"host": schema.StringAttribute{Required: true, Description: "Host name or IP."},
		"count": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(3),
			Description: "Pings per check (1-5). Default: `3`.", Validators: []validator.Int64{int64validator.Between(1, 5)}},
	}
}

func sslAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"host":            schema.StringAttribute{Required: true, Description: "Host name."},
		"port":            defaultInt("Port. Default: `443`.", 443),
		"server_name":     optString("SNI name when it differs from host."),
		"thresholds_days": computedSet("Alert this many days before expiry. Default: the server's thresholds.", types.Int64Type),
	}
}

func domainAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"domain": schema.StringAttribute{Required: true,
			Description: "Registered domain (e.g. `example.com`, not `www.example.com`: the server reduces names to the registered domain)."},
		"thresholds_days":      computedSet("Alert this many days before expiry. Default: the server's thresholds.", types.Int64Type),
		"expected_nameservers": emptySet("Expected name servers (lowercase, no trailing dot). Empty: not checked.", types.StringType),
	}
}

func pageSpeedAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"url": schema.StringAttribute{Required: true, Description: "http(s) address."},
		"max_total_ms": schema.Int64Attribute{Optional: true, Description: "The check fails when loading takes longer (ms).",
			Validators: []validator.Int64{int64validator.AtLeast(1)}},
	}
}

func pushAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"period_seconds": computedInt("Expected ping period in seconds. Default: `interval_seconds`."),
		"grace_seconds":  defaultInt("Grace period in seconds. Default: `60`.", 60),
	}
}
