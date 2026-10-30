// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/rintuphq/terraform-provider-rintup/internal/client"
)

var _ resource.ResourceWithImportState = (*contactGroupResource)(nil)

type contactGroupResource struct{ withClient }

type contactGroupModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func newContactGroupResource() resource.Resource { return &contactGroupResource{} }

func (r *contactGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_contact_group"
}

func (r *contactGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A contact group: monitors alert it, and its channels (`rintup_alert_channel`) deliver the alert. " +
			"Deleting the group deletes its channels.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name": schema.StringAttribute{Required: true, Description: "Group name (1-100 characters).",
				Validators: []validator.String{stringvalidator.LengthBetween(1, 100)}},
		},
	}
}

func (r *contactGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan contactGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	g, err := r.client.CreateContactGroup(ctx, plan.Name.ValueString())
	if err != nil {
		apiError(&resp.Diagnostics, "create contact group", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, contactGroupState(g))...)
}

func (r *contactGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior contactGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	g, err := r.client.GetContactGroup(ctx, prior.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiError(&resp.Diagnostics, "read contact group", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, contactGroupState(g))...)
}

func (r *contactGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan contactGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	g, err := r.client.UpdateContactGroup(ctx, plan.ID.ValueString(), plan.Name.ValueString())
	if err != nil {
		apiError(&resp.Diagnostics, "update contact group", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, contactGroupState(g))...)
}

func (r *contactGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var prior contactGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteIgnoringNotFound(&resp.Diagnostics, "delete contact group", r.client.DeleteContactGroup(ctx, prior.ID.ValueString()))
}

func (r *contactGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func contactGroupState(g *client.ContactGroup) contactGroupModel {
	return contactGroupModel{ID: types.StringValue(g.ID), Name: types.StringValue(g.Name)}
}
