// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/rintuphq/terraform-provider-rintup/internal/client"
)

var _ resource.ResourceWithImportState = (*maintenanceWindowResource)(nil)

type maintenanceWindowResource struct{ withClient }

type maintenanceWindowModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	MonitorIDs      types.Set    `tfsdk:"monitor_ids"`
	Recurrence      types.String `tfsdk:"recurrence"`
	StartsAt        types.String `tfsdk:"starts_at"`
	EndsAt          types.String `tfsdk:"ends_at"`
	Days            types.Set    `tfsdk:"days"`
	StartTime       types.String `tfsdk:"start_time"`
	DurationMinutes types.Int64  `tfsdk:"duration_minutes"`
	TimeZone        types.String `tfsdk:"time_zone"`
	Active          types.Bool   `tfsdk:"active"`
	NextStartsAt    types.String `tfsdk:"next_starts_at"`
	NextEndsAt      types.String `tfsdk:"next_ends_at"`
}

func newMaintenanceWindowResource() resource.Resource { return &maintenanceWindowResource{} }

func (r *maintenanceWindowResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_maintenance_window"
}

func (r *maintenanceWindowResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A maintenance window: alerts are muted for its monitors while it is active. One-off (default: starts now, " +
			"lasts 60 minutes) or recurring (`daily`, `weekly`, `monthly` at `start_time` in `time_zone`).",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name": defaultString("Name.", "Maintenance"),
			"monitor_ids": schema.SetAttribute{Required: true, ElementType: types.StringType,
				Description: "Monitors covered. An empty set means every monitor of the workspace."},
			"recurrence": defaultString("`none`, `daily`, `weekly` or `monthly`.", "none", "none", "daily", "weekly", "monthly"),
			"starts_at": schema.StringAttribute{Optional: true, Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Description:   "RFC 3339 time. One-off: start (default: now). Recurring: the series is valid from this time."},
			"ends_at": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: []planmodifier.String{endsAtModifier{}},
				Description: "RFC 3339 time. One-off: end (default: starts_at + duration). Recurring: end of the series; leave unset for no end."},
			"days": computedSet("Weekly: days 0-6 (0 = Sunday). Monthly: days of the month 1-31.", types.Int64Type),
			"start_time": schema.StringAttribute{Optional: true, Description: "Recurring: local start time `HH:mm` (required for recurring windows).",
				Validators: []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`), "must be HH:mm")}},
			"duration_minutes": computedInt("Length of each window in minutes. Default: 60."),
			"time_zone":        computedString("IANA time zone of start_time. Default: the workspace time zone, else UTC."),
			"active":           schema.BoolAttribute{Computed: true, Description: "A window is running now."},
			"next_starts_at":   schema.StringAttribute{Computed: true, Description: "Start of the running or next occurrence."},
			"next_ends_at":     schema.StringAttribute{Computed: true, Description: "End of the running or next occurrence."},
		},
	}
}

// endsAtModifier: like UseStateForUnknown, but a recurring window without ends_at in the configuration
// plans null (a series without end), so removing ends_at really clears it.
type endsAtModifier struct{}

func (endsAtModifier) Description(context.Context) string {
	return "Keeps the server value; recurring windows without ends_at have no end."
}
func (m endsAtModifier) MarkdownDescription(ctx context.Context) string { return m.Description(ctx) }

func (endsAtModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if !req.ConfigValue.IsNull() {
		return
	}
	var recurrence types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("recurrence"), &recurrence)...)
	if recurrence.IsUnknown() {
		return
	}
	if recurrence.ValueString() != "none" {
		resp.PlanValue = types.StringNull()
		return
	}
	if !req.StateValue.IsNull() {
		resp.PlanValue = req.StateValue
	}
}

func maintenanceInput(ctx context.Context, plan, prior *maintenanceWindowModel, diags *diag.Diagnostics) *client.MaintenanceWindowInput {
	in := &client.MaintenanceWindowInput{
		Name:            strPtr(plan.Name),
		StartsAt:        strPtr(plan.StartsAt),
		EndsAt:          strPtr(plan.EndsAt),
		MonitorIDs:      stringSlice(ctx, plan.MonitorIDs, diags),
		Recurrence:      strPtr(plan.Recurrence),
		Days:            int64Slice(ctx, plan.Days, diags),
		StartTime:       strPtr(plan.StartTime),
		DurationMinutes: int64Ptr(plan.DurationMinutes),
		TimeZone:        strPtr(plan.TimeZone),
	}
	if prior != nil && plan.EndsAt.IsNull() && !prior.EndsAt.IsNull() {
		in.ClearEndsAt = true
	}
	return in
}

func maintenanceState(w *client.MaintenanceWindow, prior *maintenanceWindowModel) maintenanceWindowModel {
	s := maintenanceWindowModel{
		ID:              types.StringValue(w.ID),
		Name:            types.StringValue(w.Name),
		MonitorIDs:      stringSet(w.MonitorIDs),
		Recurrence:      types.StringValue(w.Recurrence),
		StartsAt:        types.StringValue(w.StartsAt),
		EndsAt:          strVal(w.EndsAt),
		Days:            int64Set(w.Days),
		StartTime:       strVal(w.StartTime),
		DurationMinutes: int64Val(w.DurationMinutes),
		TimeZone:        strVal(w.TimeZone),
		Active:          types.BoolValue(w.Active),
		NextStartsAt:    strVal(w.NextStartsAt),
		NextEndsAt:      strVal(w.NextEndsAt),
	}
	if prior != nil { // "2026-11-01T02:00:00Z" and "2026-11-01T02:00:00+00:00" are the same instant
		s.StartsAt = keepIfSame(prior.StartsAt, s.StartsAt, sameInstant)
		s.EndsAt = keepIfSame(prior.EndsAt, s.EndsAt, sameInstant)
	}
	return s
}

func (r *maintenanceWindowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan maintenanceWindowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := maintenanceInput(ctx, &plan, nil, &resp.Diagnostics)
	w, err := r.client.CreateMaintenanceWindow(ctx, in)
	if err != nil {
		apiError(&resp.Diagnostics, "create maintenance window", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, maintenanceState(w, &plan))...)
}

func (r *maintenanceWindowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior maintenanceWindowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	w, err := r.client.GetMaintenanceWindow(ctx, prior.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiError(&resp.Diagnostics, "read maintenance window", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, maintenanceState(w, &prior))...)
}

func (r *maintenanceWindowResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior maintenanceWindowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := maintenanceInput(ctx, &plan, &prior, &resp.Diagnostics)
	w, err := r.client.UpdateMaintenanceWindow(ctx, prior.ID.ValueString(), in)
	if err != nil {
		apiError(&resp.Diagnostics, "update maintenance window", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, maintenanceState(w, &plan))...)
}

func (r *maintenanceWindowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var prior maintenanceWindowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteIgnoringNotFound(&resp.Diagnostics, "delete maintenance window", r.client.DeleteMaintenanceWindow(ctx, prior.ID.ValueString()))
}

func (r *maintenanceWindowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
