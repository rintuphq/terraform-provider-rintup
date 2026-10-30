// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/rintuphq/terraform-provider-rintup/internal/client"
)

// monitorState turns the API answer into state. Everything comes from the server (so changes made in
// the panel show up as drift) except what the API cannot return: write-only secrets stay null and their
// "_wo_version" companions are carried over from prior (plan or previous state).
func monitorState(ctx context.Context, m *client.Monitor, prior *monitorModel, diags *diag.Diagnostics) monitorModel {
	if prior == nil {
		prior = &monitorModel{}
	}
	s := monitorModel{
		ID:                  types.StringValue(m.ID),
		Type:                types.StringValue(m.Type),
		Name:                types.StringValue(m.Name),
		IntervalSeconds:     types.Int64Value(m.IntervalSeconds),
		TimeoutMs:           types.Int64Value(m.TimeoutMs),
		Locations:           stringSet(m.Locations),
		Confirmations:       types.Int64Value(m.Confirmations),
		AggregateRule:       types.StringValue(m.AggregateRule),
		TriggerDelaySeconds: types.Int64Value(m.TriggerDelaySeconds),
		RepeatEveryMinutes:  types.Int64Value(m.RepeatEveryMinutes),
		ContactGroupIDs:     stringSet(m.ContactGroupIDs),
		Tags:                stringSet(m.Tags),
		Paused:              types.BoolValue(m.Paused),
		Target:              types.StringValue(m.Target),
		Status:              types.StringValue(m.Status),
		CreatedAt:           types.StringValue(m.CreatedAt),
		PushURL:             strVal(m.PushURL),
	}
	s.HTTP = toBlock(ctx, "http", httpState(m.HTTP, asBlock[httpModel](ctx, prior.HTTP, diags), diags), diags)
	s.TCP = toBlock(ctx, "tcp", tcpState(m.TCP), diags)
	s.DNS = toBlock(ctx, "dns", dnsState(m.DNS), diags)
	s.Ping = toBlock(ctx, "ping", pingState(m.Ping), diags)
	s.SSL = toBlock(ctx, "ssl", sslState(m.SSL), diags)
	s.Domain = toBlock(ctx, "domain", domainState(m.Domain, asBlock[domainModel](ctx, prior.Domain, diags)), diags)
	s.PageSpeed = toBlock(ctx, "pagespeed", pageSpeedState(m.PageSpeed), diags)
	s.Push = toBlock(ctx, "push", pushState(m.Push), diags)
	return s
}

func httpState(h *client.HTTPSpec, prior *httpModel, diags *diag.Diagnostics) *httpModel {
	if h == nil {
		return nil
	}
	out := &httpModel{
		URL:             strVal(h.URL),
		Method:          strVal(h.Method),
		ContentType:     strVal(h.ContentType),
		BasicAuthUser:   strVal(h.BasicAuthUser),
		KeywordPresent:  strVal(h.KeywordPresent),
		KeywordAbsent:   strVal(h.KeywordAbsent),
		FollowRedirects: types.BoolValue(h.FollowRedirects == nil || *h.FollowRedirects),
		UserAgent:       strVal(h.UserAgent),
		// Write-only values are never stored.
		HeadersWO:           types.MapNull(types.StringType),
		BodyWO:              types.StringNull(),
		BasicAuthPasswordWO: types.StringNull(),
	}
	ranges := []attr.Value{}
	if h.ExpectedStatus != nil {
		for _, r := range *h.ExpectedStatus {
			v, d := types.ObjectValue(statusRangeType.AttrTypes, map[string]attr.Value{
				"from": types.Int64Value(r.From), "to": types.Int64Value(r.To)})
			diags.Append(d...)
			ranges = append(ranges, v)
		}
	}
	out.ExpectedStatus = types.ListValueMust(statusRangeType, ranges)
	if prior != nil {
		out.HeadersWOVersion = prior.HeadersWOVersion
		out.BodyWOVersion = prior.BodyWOVersion
		out.BasicAuthPasswordWOVersion = prior.BasicAuthPasswordWOVersion
	}
	return out
}

func tcpState(t *client.TCPSpec) *tcpModel {
	if t == nil {
		return nil
	}
	return &tcpModel{Host: strVal(t.Host), Port: int64Val(t.Port), Send: strVal(t.Send), Expect: strVal(t.Expect)}
}

func dnsState(d *client.DNSSpec) *dnsModel {
	if d == nil {
		return nil
	}
	return &dnsModel{Host: strVal(d.Host), RecordType: strVal(d.RecordType), Expected: stringSet(deref(d.Expected)), Resolver: strVal(d.Resolver)}
}

func pingState(p *client.PingSpec) *pingModel {
	if p == nil {
		return nil
	}
	return &pingModel{Host: strVal(p.Host), Count: int64Val(p.Count)}
}

func sslState(s *client.SSLSpec) *sslModel {
	if s == nil {
		return nil
	}
	return &sslModel{Host: strVal(s.Host), Port: int64Val(s.Port), ServerName: strVal(s.ServerName), ThresholdsDays: int64Set(deref(s.ThresholdsDays))}
}

func domainState(d *client.DomainSpec, prior *domainModel) *domainModel {
	if d == nil {
		return nil
	}
	out := &domainModel{Domain: strVal(d.Domain), ThresholdsDays: int64Set(deref(d.ThresholdsDays)),
		ExpectedNameservers: stringSet(deref(d.ExpectedNameservers))}
	if prior != nil { // the server lowercases names; "Example.com" in the configuration is not a change
		out.Domain = keepIfSame(prior.Domain, out.Domain, sameFold)
	}
	return out
}

func pageSpeedState(p *client.PageSpeedSpec) *pageSpeedModel {
	if p == nil {
		return nil
	}
	out := &pageSpeedModel{URL: strVal(p.URL), MaxTotalMs: int64Val(p.MaxTotalMs)}
	if p.MaxTotalMs != nil && *p.MaxTotalMs <= 0 {
		out.MaxTotalMs = types.Int64Null()
	}
	return out
}

func pushState(p *client.PushSpec) *pushModel {
	if p == nil {
		return nil
	}
	return &pushModel{PeriodSeconds: int64Val(p.PeriodSeconds), GraceSeconds: int64Val(p.GraceSeconds)}
}

func deref[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}
