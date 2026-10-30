// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Terraform value -> API pointer. Null and unknown both mean "not sent".

func strPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func int64Ptr(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := v.ValueInt64()
	return &i
}

func boolPtr(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

// clearableStr is for PATCH of an optional text field: a null (removed from the configuration) is sent
// as "" which the API reads as "clear"; on create (update == false) null is simply not sent.
func clearableStr(v types.String, update bool) *string {
	if v.IsNull() && update {
		empty := ""
		return &empty
	}
	return strPtr(v)
}

func stringSlice(ctx context.Context, v types.Set, diags *diag.Diagnostics) *[]string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	out := []string{}
	diags.Append(v.ElementsAs(ctx, &out, false)...)
	return &out
}

func int64Slice(ctx context.Context, v types.Set, diags *diag.Diagnostics) *[]int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	out := []int64{}
	diags.Append(v.ElementsAs(ctx, &out, false)...)
	return &out
}

// API value -> Terraform value. Empty text is null: the API clears optional text to null and
// some answers carry "" for the same meaning.

func strVal(p *string) types.String {
	if p == nil || *p == "" {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

func int64Val(p *int64) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*p)
}

func stringSet(items []string) types.Set {
	vals := make([]attr.Value, 0, len(items))
	for _, s := range items {
		vals = append(vals, types.StringValue(s))
	}
	return types.SetValueMust(types.StringType, vals)
}

func int64Set(items []int64) types.Set {
	vals := make([]attr.Value, 0, len(items))
	for _, i := range items {
		vals = append(vals, types.Int64Value(i))
	}
	return types.SetValueMust(types.Int64Type, vals)
}

func stringList(items []string) types.List {
	vals := make([]attr.Value, 0, len(items))
	for _, s := range items {
		vals = append(vals, types.StringValue(s))
	}
	return types.ListValueMust(types.StringType, vals)
}

// keepIfSame returns prior when it says the same as the server in another spelling (letter case,
// or the same instant written differently), so a normalising server does not cause plan noise.
func keepIfSame(prior types.String, server types.String, same func(a, b string) bool) types.String {
	if prior.IsNull() || prior.IsUnknown() || server.IsNull() {
		return server
	}
	if same(prior.ValueString(), server.ValueString()) {
		return prior
	}
	return server
}

func sameInstant(a, b string) bool {
	ta, errA := time.Parse(time.RFC3339Nano, a)
	tb, errB := time.Parse(time.RFC3339Nano, b)
	return errA == nil && errB == nil && ta.Equal(tb)
}

func sameFold(a, b string) bool {
	return strings.EqualFold(strings.TrimSuffix(a, "."), strings.TrimSuffix(b, "."))
}

// versionChanged reports whether a "<secret>_wo_version" attribute moved between state and plan.
func versionChanged(prior, planned types.String) bool {
	return !prior.Equal(planned)
}
