// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

// Package provider implements the RintUp Terraform provider with the HashiCorp plugin framework.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/rintuphq/terraform-provider-rintup/internal/client"
)

var _ provider.Provider = (*rintupProvider)(nil)

type rintupProvider struct {
	version string
}

type providerModel struct {
	APIKey  types.String `tfsdk:"api_key"`
	BaseURL types.String `tfsdk:"base_url"`
}

// New returns the provider factory used by main and by tests.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &rintupProvider{version: version} }
}

func (p *rintupProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "rintup"
	resp.Version = p.version
}

func (p *rintupProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage RintUp (https://rintup.com) uptime monitors, alert channels, maintenance windows and status pages as code.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				Description: "Workspace API key (rup_live_...). Create one under Settings > API keys. " +
					"Defaults to the RINTUP_API_KEY environment variable.",
			},
			"base_url": schema.StringAttribute{
				Optional:    true,
				Description: "API root. Defaults to the RINTUP_BASE_URL environment variable, else " + client.DefaultBaseURL + ".",
			},
		},
	}
}

func (p *rintupProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.APIKey.IsUnknown() || cfg.BaseURL.IsUnknown() {
		resp.Diagnostics.AddError("Unknown RintUp provider configuration",
			"api_key and base_url must be known at plan time (not computed from other resources).")
		return
	}
	apiKey := firstNonEmpty(cfg.APIKey.ValueString(), os.Getenv("RINTUP_API_KEY"))
	baseURL := firstNonEmpty(cfg.BaseURL.ValueString(), os.Getenv("RINTUP_BASE_URL"), client.DefaultBaseURL)
	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(path.Root("api_key"), "Missing RintUp API key",
			"Set api_key in the provider block or the RINTUP_API_KEY environment variable.")
		return
	}
	c := client.New(baseURL, apiKey, p.version)
	// Only method, path and status are logged: never headers, bodies or query strings.
	c.OnRequest = func(ctx context.Context, method, reqPath string, status int) {
		tflog.Debug(ctx, "rintup api", map[string]any{"method": method, "path": reqPath, "status": status})
	}
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *rintupProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newMonitorResource,
		newContactGroupResource,
		newAlertChannelResource,
		newMaintenanceWindowResource,
		newStatusPageResource,
	}
}

func (p *rintupProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newLocationsDataSource,
		newMonitorDataSource,
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
