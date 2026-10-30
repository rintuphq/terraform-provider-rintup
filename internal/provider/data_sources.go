// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/rintuphq/terraform-provider-rintup/internal/client"
)

// rintup_locations ---------------------------------------------------------------------------------

type locationsDataSource struct{ dataSourceWithClient }

var locationType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"code": types.StringType, "name": types.StringType, "country": types.StringType, "online": types.BoolType,
	"ipv4": types.ListType{ElemType: types.StringType}, "ipv6": types.ListType{ElemType: types.StringType},
}}

type locationsModel struct {
	Locations types.List `tfsdk:"locations"`
	Codes     types.List `tfsdk:"codes"`
}

func newLocationsDataSource() datasource.DataSource { return &locationsDataSource{} }

func (d *locationsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_locations"
}

func (d *locationsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	str := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, Description: desc}
	}
	resp.Schema = schema.Schema{
		Description: "Probe locations, ordered by code. Their fixed egress addresses can be allow-listed in firewalls.",
		Attributes: map[string]schema.Attribute{
			"codes": schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "All location codes."},
			"locations": schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"code":    str("Location code, used in `rintup_monitor.locations`."),
					"name":    str("City / region name."),
					"country": str("Country code."),
					"online":  schema.BoolAttribute{Computed: true, Description: "The probe reported recently."},
					"ipv4":    schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "Fixed IPv4 egress addresses."},
					"ipv6":    schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "Fixed IPv6 egress addresses."},
				},
			}},
		},
	}
}

func (d *locationsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	locs, err := d.client.ListLocations(ctx)
	if err != nil {
		apiError(&resp.Diagnostics, "list locations", err)
		return
	}
	items := make([]attr.Value, 0, len(locs))
	codes := make([]string, 0, len(locs))
	for _, l := range locs {
		v, diags := types.ObjectValue(locationType.AttrTypes, map[string]attr.Value{
			"code": types.StringValue(l.Code), "name": types.StringValue(l.Name), "country": types.StringValue(l.Country),
			"online": types.BoolValue(l.Online), "ipv4": stringList(l.IPv4), "ipv6": stringList(l.IPv6)})
		resp.Diagnostics.Append(diags...)
		items = append(items, v)
		codes = append(codes, l.Code)
	}
	state := locationsModel{Locations: types.ListValueMust(locationType, items), Codes: stringList(codes)}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// rintup_monitor (data) ------------------------------------------------------------------------------

type monitorDataSource struct{ dataSourceWithClient }

type monitorDataModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Type            types.String `tfsdk:"type"`
	Target          types.String `tfsdk:"target"`
	Status          types.String `tfsdk:"status"`
	IntervalSeconds types.Int64  `tfsdk:"interval_seconds"`
	Paused          types.Bool   `tfsdk:"paused"`
	Locations       types.Set    `tfsdk:"locations"`
	Tags            types.Set    `tfsdk:"tags"`
}

var _ datasource.DataSourceWithConfigValidators = (*monitorDataSource)(nil)

func newMonitorDataSource() datasource.DataSource { return &monitorDataSource{} }

func (d *monitorDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor"
}

func (d *monitorDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	computed := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, Description: desc}
	}
	resp.Schema = schema.Schema{
		Description: "Looks up an existing monitor by exact name (or by id), e.g. to put a monitor managed elsewhere on a status page.",
		Attributes: map[string]schema.Attribute{
			"id":               schema.StringAttribute{Optional: true, Computed: true, Description: "Monitor id. Set this or `name`."},
			"name":             schema.StringAttribute{Optional: true, Computed: true, Description: "Exact monitor name. It must match exactly one monitor."},
			"type":             computed("Monitor type."),
			"target":           computed("What is checked."),
			"status":           computed("Current status."),
			"interval_seconds": schema.Int64Attribute{Computed: true},
			"paused":           schema.BoolAttribute{Computed: true},
			"locations":        schema.SetAttribute{Computed: true, ElementType: types.StringType},
			"tags":             schema.SetAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *monitorDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name"))}
}

func (d *monitorDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg monitorDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var item client.MonitorListItem
	if !cfg.ID.IsNull() {
		m, err := d.client.GetMonitor(ctx, cfg.ID.ValueString())
		if err != nil {
			apiError(&resp.Diagnostics, "read monitor", err)
			return
		}
		item = client.MonitorListItem{ID: m.ID, Name: m.Name, Type: m.Type, Target: m.Target, Status: m.Status,
			IntervalSeconds: m.IntervalSeconds, Paused: m.Paused, Locations: m.Locations, Tags: m.Tags}
	} else {
		name := cfg.Name.ValueString()
		list, err := d.client.ListMonitors(ctx, name) // q narrows on the server; exact match here
		if err != nil {
			apiError(&resp.Diagnostics, "list monitors", err)
			return
		}
		var matches []client.MonitorListItem
		for _, m := range list {
			if m.Name == name {
				matches = append(matches, m)
			}
		}
		if len(matches) != 1 {
			resp.Diagnostics.AddAttributeError(path.Root("name"), "Monitor not found",
				fmt.Sprintf("%d monitors are named %q; exactly one is needed.",
					len(matches), name))
			return
		}
		item = matches[0]
	}
	state := monitorDataModel{ID: types.StringValue(item.ID), Name: types.StringValue(item.Name), Type: types.StringValue(item.Type),
		Target: types.StringValue(item.Target), Status: types.StringValue(item.Status), IntervalSeconds: types.Int64Value(item.IntervalSeconds),
		Paused: types.BoolValue(item.Paused), Locations: stringSet(item.Locations), Tags: stringSet(item.Tags)}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
