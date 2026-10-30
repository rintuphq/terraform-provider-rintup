// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/rintuphq/terraform-provider-rintup/internal/client"
)

type monitorModel struct {
	ID                  types.String `tfsdk:"id"`
	Type                types.String `tfsdk:"type"`
	Name                types.String `tfsdk:"name"`
	IntervalSeconds     types.Int64  `tfsdk:"interval_seconds"`
	TimeoutMs           types.Int64  `tfsdk:"timeout_ms"`
	Locations           types.Set    `tfsdk:"locations"`
	Confirmations       types.Int64  `tfsdk:"confirmations"`
	AggregateRule       types.String `tfsdk:"aggregate_rule"`
	TriggerDelaySeconds types.Int64  `tfsdk:"trigger_delay_seconds"`
	RepeatEveryMinutes  types.Int64  `tfsdk:"repeat_every_minutes"`
	ContactGroupIDs     types.Set    `tfsdk:"contact_group_ids"`
	Tags                types.Set    `tfsdk:"tags"`
	Paused              types.Bool   `tfsdk:"paused"`
	Target              types.String `tfsdk:"target"`
	Status              types.String `tfsdk:"status"`
	CreatedAt           types.String `tfsdk:"created_at"`
	PushURL             types.String `tfsdk:"push_url"`
	HTTP                types.Object `tfsdk:"http"`
	TCP                 types.Object `tfsdk:"tcp"`
	DNS                 types.Object `tfsdk:"dns"`
	Ping                types.Object `tfsdk:"ping"`
	SSL                 types.Object `tfsdk:"ssl"`
	Domain              types.Object `tfsdk:"domain"`
	PageSpeed           types.Object `tfsdk:"pagespeed"`
	Push                types.Object `tfsdk:"push"`
}

type httpModel struct {
	URL                        types.String `tfsdk:"url"`
	Method                     types.String `tfsdk:"method"`
	ContentType                types.String `tfsdk:"content_type"`
	BasicAuthUser              types.String `tfsdk:"basic_auth_user"`
	KeywordPresent             types.String `tfsdk:"keyword_present"`
	KeywordAbsent              types.String `tfsdk:"keyword_absent"`
	FollowRedirects            types.Bool   `tfsdk:"follow_redirects"`
	UserAgent                  types.String `tfsdk:"user_agent"`
	ExpectedStatus             types.List   `tfsdk:"expected_status"`
	HeadersWO                  types.Map    `tfsdk:"headers_wo"`
	HeadersWOVersion           types.String `tfsdk:"headers_wo_version"`
	BodyWO                     types.String `tfsdk:"body_wo"`
	BodyWOVersion              types.String `tfsdk:"body_wo_version"`
	BasicAuthPasswordWO        types.String `tfsdk:"basic_auth_password_wo"`
	BasicAuthPasswordWOVersion types.String `tfsdk:"basic_auth_password_wo_version"`
}

type statusRangeModel struct {
	From types.Int64 `tfsdk:"from"`
	To   types.Int64 `tfsdk:"to"`
}

type tcpModel struct {
	Host   types.String `tfsdk:"host"`
	Port   types.Int64  `tfsdk:"port"`
	Send   types.String `tfsdk:"send"`
	Expect types.String `tfsdk:"expect"`
}

type dnsModel struct {
	Host       types.String `tfsdk:"host"`
	RecordType types.String `tfsdk:"record_type"`
	Expected   types.Set    `tfsdk:"expected"`
	Resolver   types.String `tfsdk:"resolver"`
}

type pingModel struct {
	Host  types.String `tfsdk:"host"`
	Count types.Int64  `tfsdk:"count"`
}

type sslModel struct {
	Host           types.String `tfsdk:"host"`
	Port           types.Int64  `tfsdk:"port"`
	ServerName     types.String `tfsdk:"server_name"`
	ThresholdsDays types.Set    `tfsdk:"thresholds_days"`
}

type domainModel struct {
	Domain              types.String `tfsdk:"domain"`
	ThresholdsDays      types.Set    `tfsdk:"thresholds_days"`
	ExpectedNameservers types.Set    `tfsdk:"expected_nameservers"`
}

type pageSpeedModel struct {
	URL        types.String `tfsdk:"url"`
	MaxTotalMs types.Int64  `tfsdk:"max_total_ms"`
}

type pushModel struct {
	PeriodSeconds types.Int64 `tfsdk:"period_seconds"`
	GraceSeconds  types.Int64 `tfsdk:"grace_seconds"`
}

// blockTypes are the object types of the per-type blocks, taken from the schema (single source).
var blockTypes = func() map[string]map[string]attr.Type {
	out := map[string]map[string]attr.Type{}
	for name, a := range monitorSchema().Attributes {
		if ot, ok := a.GetType().(basetypes.ObjectType); ok {
			out[name] = ot.AttrTypes
		}
	}
	return out
}()

// asBlock decodes a block; nil when it is null or unknown.
func asBlock[T any](ctx context.Context, o types.Object, diags *diag.Diagnostics) *T {
	if o.IsNull() || o.IsUnknown() {
		return nil
	}
	var v T
	diags.Append(o.As(ctx, &v, basetypes.ObjectAsOptions{})...)
	return &v
}

func toBlock[T any](ctx context.Context, name string, v *T, diags *diag.Diagnostics) types.Object {
	if v == nil {
		return types.ObjectNull(blockTypes[name])
	}
	o, d := types.ObjectValueFrom(ctx, blockTypes[name], v)
	diags.Append(d...)
	return o
}

// monitorInput builds the POST (prior == nil) or PATCH body. plan holds the planned values, config the
// raw configuration: write-only secrets exist only there. On PATCH a secret is sent only when its
// "_wo_version" changed; a removed secret is then cleared ("" / {}).
func monitorInput(ctx context.Context, plan, config, prior *monitorModel, diags *diag.Diagnostics) *client.MonitorInput {
	update := prior != nil
	in := &client.MonitorInput{
		Name:                strPtr(plan.Name),
		IntervalSeconds:     int64Ptr(plan.IntervalSeconds),
		TimeoutMs:           int64Ptr(plan.TimeoutMs),
		Locations:           stringSlice(ctx, plan.Locations, diags),
		Confirmations:       int64Ptr(plan.Confirmations),
		AggregateRule:       strPtr(plan.AggregateRule),
		TriggerDelaySeconds: int64Ptr(plan.TriggerDelaySeconds),
		RepeatEveryMinutes:  int64Ptr(plan.RepeatEveryMinutes),
		ContactGroupIDs:     stringSlice(ctx, plan.ContactGroupIDs, diags),
		Tags:                stringSlice(ctx, plan.Tags, diags),
	}
	if !update {
		in.Type = strPtr(plan.Type) // the type cannot change; PATCH leaves it out
	}
	if h := asBlock[httpModel](ctx, plan.HTTP, diags); h != nil {
		var cfg, old *httpModel
		cfg = asBlock[httpModel](ctx, config.HTTP, diags)
		if update {
			old = asBlock[httpModel](ctx, prior.HTTP, diags)
		}
		in.HTTP = httpInput(ctx, h, cfg, old, update, diags)
	}
	if t := asBlock[tcpModel](ctx, plan.TCP, diags); t != nil {
		in.TCP = &client.TCPSpec{Host: strPtr(t.Host), Port: int64Ptr(t.Port),
			Send: clearableStr(t.Send, update), Expect: clearableStr(t.Expect, update)}
	}
	if d := asBlock[dnsModel](ctx, plan.DNS, diags); d != nil {
		in.DNS = &client.DNSSpec{Host: strPtr(d.Host), RecordType: strPtr(d.RecordType),
			Expected: stringSlice(ctx, d.Expected, diags), Resolver: clearableStr(d.Resolver, update)}
	}
	if p := asBlock[pingModel](ctx, plan.Ping, diags); p != nil {
		in.Ping = &client.PingSpec{Host: strPtr(p.Host), Count: int64Ptr(p.Count)}
	}
	if s := asBlock[sslModel](ctx, plan.SSL, diags); s != nil {
		in.SSL = &client.SSLSpec{Host: strPtr(s.Host), Port: int64Ptr(s.Port),
			ServerName: clearableStr(s.ServerName, update), ThresholdsDays: int64Slice(ctx, s.ThresholdsDays, diags)}
	}
	if d := asBlock[domainModel](ctx, plan.Domain, diags); d != nil {
		in.Domain = &client.DomainSpec{Domain: strPtr(d.Domain), ThresholdsDays: int64Slice(ctx, d.ThresholdsDays, diags),
			ExpectedNameservers: stringSlice(ctx, d.ExpectedNameservers, diags)}
	}
	if p := asBlock[pageSpeedModel](ctx, plan.PageSpeed, diags); p != nil {
		in.PageSpeed = &client.PageSpeedSpec{URL: strPtr(p.URL), MaxTotalMs: int64Ptr(p.MaxTotalMs)}
		if update && p.MaxTotalMs.IsNull() {
			zero := int64(0) // the API stores 0 as "no threshold"
			in.PageSpeed.MaxTotalMs = &zero
		}
	}
	if p := asBlock[pushModel](ctx, plan.Push, diags); p != nil {
		in.Push = &client.PushSpec{PeriodSeconds: int64Ptr(p.PeriodSeconds), GraceSeconds: int64Ptr(p.GraceSeconds)}
	}
	return in
}

func httpInput(ctx context.Context, h, cfg, old *httpModel, update bool, diags *diag.Diagnostics) *client.HTTPSpec {
	out := &client.HTTPSpec{
		URL: strPtr(h.URL), Method: strPtr(h.Method), FollowRedirects: boolPtr(h.FollowRedirects),
		ContentType: clearableStr(h.ContentType, update), BasicAuthUser: clearableStr(h.BasicAuthUser, update),
		KeywordPresent: clearableStr(h.KeywordPresent, update), KeywordAbsent: clearableStr(h.KeywordAbsent, update),
		UserAgent: clearableStr(h.UserAgent, update),
	}
	if !h.ExpectedStatus.IsNull() && !h.ExpectedStatus.IsUnknown() {
		var ranges []statusRangeModel
		diags.Append(h.ExpectedStatus.ElementsAs(ctx, &ranges, false)...)
		list := make([]client.StatusRange, 0, len(ranges))
		for _, r := range ranges {
			list = append(list, client.StatusRange{From: r.From.ValueInt64(), To: r.To.ValueInt64()})
		}
		out.ExpectedStatus = &list
	}
	if cfg == nil {
		cfg = &httpModel{}
	}
	send := func(prior, planned types.String) bool {
		return !update || old == nil || versionChanged(prior, planned)
	}
	var prev httpModel
	if old != nil {
		prev = *old
	}
	if send(prev.HeadersWOVersion, h.HeadersWOVersion) && (update || !cfg.HeadersWO.IsNull()) {
		headers := map[string]string{} // {} on update clears every header
		if !cfg.HeadersWO.IsNull() && !cfg.HeadersWO.IsUnknown() {
			diags.Append(cfg.HeadersWO.ElementsAs(ctx, &headers, false)...)
		}
		out.Headers = &headers
	}
	if send(prev.BodyWOVersion, h.BodyWOVersion) {
		out.Body = clearableStr(cfg.BodyWO, update)
	}
	if send(prev.BasicAuthPasswordWOVersion, h.BasicAuthPasswordWOVersion) {
		out.BasicAuthPassword = clearableStr(cfg.BasicAuthPasswordWO, update)
	}
	return out
}
