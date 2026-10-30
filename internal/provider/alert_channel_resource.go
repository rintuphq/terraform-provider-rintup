// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/rintuphq/terraform-provider-rintup/internal/client"
)

var _ resource.ResourceWithImportState = (*alertChannelResource)(nil)

// importedKey marks an imported channel in private state. The API never returns a channel's target,
// secret or locale, so right after import those come from the configuration without a replacement.
const importedKey = "imported"

var channelTypes = []string{"email", "sms", "webhook", "slack", "discord", "telegram", "teams", "pagerduty", "opsgenie"}

type alertChannelResource struct{ withClient }

type alertChannelModel struct {
	ID                   types.String `tfsdk:"id"`
	ContactGroupID       types.String `tfsdk:"contact_group_id"`
	Type                 types.String `tfsdk:"type"`
	Target               types.String `tfsdk:"target"`
	TargetWO             types.String `tfsdk:"target_wo"`
	TargetWOVersion      types.String `tfsdk:"target_wo_version"`
	SecretWO             types.String `tfsdk:"secret_wo"`
	SecretWOVersion      types.String `tfsdk:"secret_wo_version"`
	Locale               types.String `tfsdk:"locale"`
	Display              types.String `tfsdk:"display"`
	Verified             types.Bool   `tfsdk:"verified"`
	RequiresVerification types.Bool   `tfsdk:"requires_verification"`
	GeneratedSecret      types.String `tfsdk:"generated_secret"`
}

func newAlertChannelResource() resource.Resource { return &alertChannelResource{} }

func (r *alertChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_channel"
}

// replaceUnlessImported: the API cannot update a channel, so every change replaces it, except the
// first plan after an import, where the configuration is adopted as is.
func replaceUnlessImported() planmodifier.String {
	return stringplanmodifier.RequiresReplaceIf(func(ctx context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
		flag, _ := req.Private.GetKey(ctx, importedKey)
		resp.RequiresReplace = len(flag) == 0
	}, "Changing this replaces the channel (the API has no channel update).", "Changing this replaces the channel (the API has no channel update).")
}

func (r *alertChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{replaceUnlessImported()}
	resp.Schema = schema.Schema{
		Description: "A notification channel in a contact group (e-mail, SMS, webhook, Slack, Discord, Telegram, Teams, PagerDuty, Opsgenie). " +
			"The API cannot change a channel, so any change replaces it. E-mail and SMS channels start unverified: " +
			"the target receives a verification link or code.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"contact_group_id": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Description: "The `rintup_contact_group` this channel belongs to."},
			"type": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:  []validator.String{stringvalidator.OneOf(channelTypes...)},
				Description: "`email`, `sms`, `webhook`, `slack`, `discord`, `telegram`, `teams`, `pagerduty` or `opsgenie`."},
			"target": schema.StringAttribute{Optional: true, PlanModifiers: replace,
				Validators: []validator.String{stringvalidator.ConflictsWith(path.MatchRoot("target_wo"))},
				Description: "Non-secret target: e-mail address, phone number (E.164 or TR national), Telegram chat id, " +
					"Opsgenie region (`us`/`eu`). For webhook URLs that act as credentials (Slack, Discord, Teams) use `target_wo`."},
			"target_wo": schema.StringAttribute{Optional: true, Sensitive: true, WriteOnly: true,
				Description: "Secret target, e.g. a Slack/Discord/Teams incoming webhook URL. Write-only: never stored in state. " +
					"Change `target_wo_version` to send a new value (replaces the channel)."},
			"target_wo_version": schema.StringAttribute{Optional: true, PlanModifiers: replace,
				Description: "Any value; when it changes the channel is recreated with the current `target_wo`."},
			"secret_wo": schema.StringAttribute{Optional: true, Sensitive: true, WriteOnly: true,
				Description: "Webhook signing key, Telegram bot token, PagerDuty routing key or Opsgenie API key. " +
					"Write-only: never stored in state. Change `secret_wo_version` to send a new value (replaces the channel)."},
			"secret_wo_version": schema.StringAttribute{Optional: true, PlanModifiers: replace,
				Description: "Any value; when it changes the channel is recreated with the current `secret_wo`."},
			"locale": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("en"),
				PlanModifiers: replace, Validators: []validator.String{stringvalidator.OneOf("en", "tr")},
				Description: "Notification language: `en` or `tr`. Default: `en`."},
			"display": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Description: "Masked target as the API shows it."},
			"verified": schema.BoolAttribute{Computed: true,
				Description: "E-mail/SMS: the target was verified. Other types: a notification reached it."},
			"requires_verification": schema.BoolAttribute{Computed: true},
			"generated_secret": schema.StringAttribute{Computed: true, Sensitive: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Description: "Webhook channels without `secret_wo`: the signing key the server generated. The API returns it " +
					"only once, so it is kept in state (sensitive). Set `secret_wo` to keep state free of it."},
		},
	}
}

func (r *alertChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg alertChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	target := strPtr(plan.Target)
	if target == nil {
		target = strPtr(cfg.TargetWO)
	}
	in := &client.ChannelInput{Type: plan.Type.ValueString(), Target: target, Secret: strPtr(cfg.SecretWO), Locale: strPtr(plan.Locale)}
	ch, err := r.client.CreateChannel(ctx, plan.ContactGroupID.ValueString(), in)
	if err != nil {
		apiError(&resp.Diagnostics, "create alert channel", err)
		return
	}
	plan.GeneratedSecret = strVal(ch.Secret)
	channelState(&plan, ch)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *alertChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state alertChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ch, groupID, err := r.client.FindChannel(ctx, state.ContactGroupID.ValueString(), state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx) // the channel or its whole group was deleted
		return
	}
	if err != nil {
		apiError(&resp.Diagnostics, "read alert channel", err)
		return
	}
	state.ContactGroupID = types.StringValue(groupID)
	channelState(&state, ch)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update only runs right after an import (every other change replaces the channel): it adopts the
// configured target/locale/versions into state and clears the import marker.
func (r *alertChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state alertChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Display, plan.Verified, plan.RequiresVerification = state.Display, state.Verified, state.RequiresVerification
	plan.GeneratedSecret = state.GeneratedSecret
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, importedKey, nil)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *alertChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state alertChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteIgnoringNotFound(&resp.Diagnostics, "delete alert channel", r.client.DeleteChannel(ctx, state.ID.ValueString()))
}

// ImportState accepts "<contact_group_id>/<channel_id>" or just "<channel_id>" (all groups are searched).
func (r *alertChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if group, channel, ok := strings.Cut(req.ID, "/"); ok {
		id = channel
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("contact_group_id"), group)...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, importedKey, []byte("true"))...)
}

func channelState(m *alertChannelModel, ch *client.Channel) {
	m.ID = types.StringValue(ch.ID)
	m.Type = types.StringValue(ch.Type)
	m.Display = types.StringValue(ch.Display)
	m.Verified = types.BoolValue(ch.Verified)
	m.RequiresVerification = types.BoolValue(ch.RequiresVerification)
	m.TargetWO = types.StringNull()
	m.SecretWO = types.StringNull()
}
