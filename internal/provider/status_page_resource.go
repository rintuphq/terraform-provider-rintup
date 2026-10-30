// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/rintuphq/terraform-provider-rintup/internal/client"
)

var _ resource.ResourceWithImportState = (*statusPageResource)(nil)

var componentType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"monitor_id": types.StringType, "display_name": types.StringType, "group": types.StringType}}

type statusPageResource struct{ withClient }

type statusPageModel struct {
	ID                        types.String `tfsdk:"id"`
	Title                     types.String `tfsdk:"title"`
	Slug                      types.String `tfsdk:"slug"`
	Description               types.String `tfsdk:"description"`
	LogoURL                   types.String `tfsdk:"logo_url"`
	BrandColor                types.String `tfsdk:"brand_color"`
	ShowResponseTimes         types.Bool   `tfsdk:"show_response_times"`
	Noindex                   types.Bool   `tfsdk:"noindex"`
	Locale                    types.String `tfsdk:"locale"`
	CustomDomain              types.String `tfsdk:"custom_domain"`
	AllowSubscriptions        types.Bool   `tfsdk:"allow_subscriptions"`
	NotifySubscribersOnOutage types.Bool   `tfsdk:"notify_subscribers_on_outage"`
	Components                types.List   `tfsdk:"components"`
	PasswordWO                types.String `tfsdk:"password_wo"`
	PasswordWOVersion         types.String `tfsdk:"password_wo_version"`
	PasswordProtected         types.Bool   `tfsdk:"password_protected"`
	CustomDomainVerified      types.Bool   `tfsdk:"custom_domain_verified"`
}

type componentModel struct {
	MonitorID   types.String `tfsdk:"monitor_id"`
	DisplayName types.String `tfsdk:"display_name"`
	Group       types.String `tfsdk:"group"`
}

func newStatusPageResource() resource.Resource { return &statusPageResource{} }

func (r *statusPageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page"
}

func boolDefault(desc string, def bool) schema.BoolAttribute {
	return schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(def), Description: desc}
}

func (r *statusPageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"title": schema.StringAttribute{Required: true, Description: "Page title (1-120 characters).",
			Validators: []validator.String{stringvalidator.LengthBetween(1, 120)}},
		"slug":                         computedString("Address: status.rintup.com/{slug}; 3-48 lowercase letters, digits and dashes. Default: derived from the title."),
		"description":                  optString("Short description on the page."),
		"logo_url":                     optString("Logo address (https)."),
		"brand_color":                  optString("Brand colour `#RRGGBB`."),
		"custom_domain":                optString("Custom domain such as status.example.com (plan feature; verified with a TXT record)."),
		"show_response_times":          boolDefault("Show response times. Default: `true`.", true),
		"noindex":                      boolDefault("Ask search engines not to index the page. Default: `false`.", false),
		"locale":                       defaultString("Page language: `en` or `tr`.", "en", "en", "tr"),
		"allow_subscriptions":          boolDefault("Visitors may subscribe by e-mail (double opt-in). Default: `false`.", false),
		"notify_subscribers_on_outage": boolDefault("E-mail subscribers automatically when a component goes down or recovers. Default: `false`.", false),
		"components": schema.ListNestedAttribute{Optional: true, Computed: true,
			Description: "Components in display order; each shows one monitor. The list replaces the page's components as a whole.",
			Default:     listdefault.StaticValue(types.ListValueMust(componentType, []attr.Value{})),
			NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"monitor_id":   schema.StringAttribute{Required: true, Description: "Monitor id (same workspace)."},
				"display_name": schema.StringAttribute{Optional: true, Computed: true, Description: "Name on the page. Default: the monitor's name."},
				"group":        optString("Group heading."),
			}}},
		"password_protected":     schema.BoolAttribute{Computed: true, Description: "The page asks visitors for a password."},
		"custom_domain_verified": schema.BoolAttribute{Computed: true, Description: "The custom domain's TXT record was verified."},
	}
	secretPair(attrs, "password_wo", "Page password: visitors must enter it. Removing it together with its version removes the protection.")
	resp.Schema = schema.Schema{Description: "A public status page.", Attributes: attrs}
}

func statusPageInput(ctx context.Context, p *statusPageModel, update bool, diags *diag.Diagnostics) *client.StatusPageInput {
	in := &client.StatusPageInput{
		Title: strPtr(p.Title), Slug: strPtr(p.Slug),
		Description: clearableStr(p.Description, update), LogoURL: clearableStr(p.LogoURL, update),
		BrandColor: clearableStr(p.BrandColor, update), CustomDomain: clearableStr(p.CustomDomain, update),
		ShowResponseTimes: boolPtr(p.ShowResponseTimes), Noindex: boolPtr(p.Noindex), Locale: strPtr(p.Locale),
		AllowSubscriptions: boolPtr(p.AllowSubscriptions), NotifySubscribersOnOutage: boolPtr(p.NotifySubscribersOnOutage),
	}
	if !p.Components.IsNull() && !p.Components.IsUnknown() {
		var comps []componentModel
		diags.Append(p.Components.ElementsAs(ctx, &comps, false)...)
		list := make([]client.Component, 0, len(comps))
		for _, c := range comps {
			list = append(list, client.Component{MonitorID: c.MonitorID.ValueString(), DisplayName: strPtr(c.DisplayName), Group: strPtr(c.Group)})
		}
		in.Components = &list
	}
	return in
}

func statusPageState(sp *client.StatusPage, prior *statusPageModel, diags *diag.Diagnostics) statusPageModel {
	comps := make([]attr.Value, 0, len(sp.Components))
	for _, c := range sp.Components {
		v, d := types.ObjectValue(componentType.AttrTypes, map[string]attr.Value{
			"monitor_id": types.StringValue(c.MonitorID), "display_name": strVal(c.DisplayName), "group": strVal(c.Group)})
		diags.Append(d...)
		comps = append(comps, v)
	}
	return statusPageModel{
		ID: types.StringValue(sp.ID), Title: types.StringValue(sp.Title), Slug: types.StringValue(sp.Slug),
		Description: strVal(sp.Description), LogoURL: strVal(sp.LogoURL), BrandColor: keepIfSame(prior.BrandColor, strVal(sp.BrandColor), sameFold),
		CustomDomain:      keepIfSame(prior.CustomDomain, strVal(sp.CustomDomain), sameFold),
		ShowResponseTimes: types.BoolValue(sp.ShowResponseTimes), Noindex: types.BoolValue(sp.Noindex),
		Locale: types.StringValue(sp.Locale), AllowSubscriptions: types.BoolValue(sp.AllowSubscriptions),
		NotifySubscribersOnOutage: types.BoolValue(sp.NotifySubscribersOnOutage),
		Components:                types.ListValueMust(componentType, comps),
		PasswordWO:                types.StringNull(), // write-only: never stored
		PasswordWOVersion:         prior.PasswordWOVersion,
		PasswordProtected:         types.BoolValue(sp.PasswordProtected),
		CustomDomainVerified:      types.BoolValue(sp.CustomDomainVerified),
	}
}

// applyPassword sets or removes the password when its version changed (always on create when given).
func (r *statusPageResource) applyPassword(ctx context.Context, id string, cfg, plan, prior *statusPageModel) (bool, error) {
	if prior == nil {
		if cfg.PasswordWO.IsNull() {
			return false, nil
		}
		return true, r.client.SetStatusPagePassword(ctx, id, cfg.PasswordWO.ValueString())
	}
	if !versionChanged(prior.PasswordWOVersion, plan.PasswordWOVersion) {
		return false, nil
	}
	if cfg.PasswordWO.IsNull() {
		return true, r.client.RemoveStatusPagePassword(ctx, id)
	}
	return true, r.client.SetStatusPagePassword(ctx, id, cfg.PasswordWO.ValueString())
}

func (r *statusPageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg statusPageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sp, err := r.client.CreateStatusPage(ctx, statusPageInput(ctx, &plan, false, &resp.Diagnostics))
	if err != nil {
		apiError(&resp.Diagnostics, "create status page", err)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), sp.ID)...)
	r.finish(ctx, sp, &cfg, &plan, nil, &resp.State, &resp.Diagnostics)
}

func (r *statusPageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior statusPageModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sp, err := r.client.GetStatusPage(ctx, prior.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiError(&resp.Diagnostics, "read status page", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, statusPageState(sp, &prior, &resp.Diagnostics))...)
}

func (r *statusPageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, cfg, prior statusPageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sp, err := r.client.UpdateStatusPage(ctx, prior.ID.ValueString(), statusPageInput(ctx, &plan, true, &resp.Diagnostics))
	if err != nil {
		apiError(&resp.Diagnostics, "update status page", err)
		return
	}
	r.finish(ctx, sp, &cfg, &plan, &prior, &resp.State, &resp.Diagnostics)
}

// finish applies the password, re-reads the page when it changed and stores the state.
func (r *statusPageResource) finish(ctx context.Context, sp *client.StatusPage, cfg, plan, prior *statusPageModel, state *tfsdk.State, diags *diag.Diagnostics) {
	changed, err := r.applyPassword(ctx, sp.ID, cfg, plan, prior)
	if err != nil {
		apiError(diags, "set status page password", err)
		return
	}
	if changed {
		if sp, err = r.client.GetStatusPage(ctx, sp.ID); err != nil {
			apiError(diags, "read status page", err)
			return
		}
	}
	diags.Append(state.Set(ctx, statusPageState(sp, plan, diags))...)
}

func (r *statusPageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var prior statusPageModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteIgnoringNotFound(&resp.Diagnostics, "delete status page", r.client.DeleteStatusPage(ctx, prior.ID.ValueString()))
}

func (r *statusPageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
