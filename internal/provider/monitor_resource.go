// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/rintuphq/terraform-provider-rintup/internal/client"
)

var (
	_ resource.ResourceWithImportState    = (*monitorResource)(nil)
	_ resource.ResourceWithValidateConfig = (*monitorResource)(nil)
)

type monitorResource struct{ withClient }

func newMonitorResource() resource.Resource { return &monitorResource{} }

func (r *monitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor"
}

func (r *monitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = monitorSchema()
}

// ValidateConfig: exactly the block that matches type (push may omit its block).
func (r *monitorResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg monitorModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.Type.IsUnknown() || cfg.Type.IsNull() {
		return
	}
	kind := cfg.Type.ValueString()
	blocks := map[string]types.Object{"http": cfg.HTTP, "tcp": cfg.TCP, "dns": cfg.DNS, "ping": cfg.Ping,
		"ssl": cfg.SSL, "domain": cfg.Domain, "pagespeed": cfg.PageSpeed, "push": cfg.Push}
	for name, block := range blocks {
		set := !block.IsNull()
		switch {
		case name == kind && !set && kind != "push":
			resp.Diagnostics.AddAttributeError(path.Root(name), "Missing "+name+" block",
				"type = \""+kind+"\" needs the "+name+" block with its target.")
		case name != kind && set:
			resp.Diagnostics.AddAttributeError(path.Root(name), "Block does not match type",
				"The "+name+" block is only valid when type = \""+name+"\".")
		}
	}
}

func (r *monitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg monitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := monitorInput(ctx, &plan, &cfg, nil, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	m, err := r.client.CreateMonitor(ctx, in)
	if err != nil {
		apiError(&resp.Diagnostics, "create monitor", err)
		return
	}
	// Save the id first: if pausing fails the monitor still exists and must be tracked.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), m.ID)...)
	if m, err = r.applyPaused(ctx, m, plan.Paused); err != nil {
		apiError(&resp.Diagnostics, "pause monitor", err)
		return
	}
	state := monitorState(ctx, m, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *monitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior monitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	m, err := r.client.GetMonitor(ctx, prior.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx) // deleted outside Terraform: plan will create it again
		return
	}
	if err != nil {
		apiError(&resp.Diagnostics, "read monitor", err)
		return
	}
	state := monitorState(ctx, m, &prior, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *monitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, cfg, prior monitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := monitorInput(ctx, &plan, &cfg, &prior, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	m, err := r.client.UpdateMonitor(ctx, prior.ID.ValueString(), in)
	if err != nil {
		apiError(&resp.Diagnostics, "update monitor", err)
		return
	}
	if m, err = r.applyPaused(ctx, m, plan.Paused); err != nil {
		apiError(&resp.Diagnostics, "pause/resume monitor", err)
		return
	}
	state := monitorState(ctx, m, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *monitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var prior monitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteIgnoringNotFound(&resp.Diagnostics, "delete monitor", r.client.DeleteMonitor(ctx, prior.ID.ValueString()))
}

func (r *monitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// applyPaused pauses or resumes when the server state differs from the plan, then re-reads.
func (r *monitorResource) applyPaused(ctx context.Context, m *client.Monitor, want types.Bool) (*client.Monitor, error) {
	if want.IsNull() || want.IsUnknown() || want.ValueBool() == m.Paused {
		return m, nil
	}
	if err := r.client.SetMonitorPaused(ctx, m.ID, want.ValueBool()); err != nil {
		return m, err
	}
	return r.client.GetMonitor(ctx, m.ID)
}
