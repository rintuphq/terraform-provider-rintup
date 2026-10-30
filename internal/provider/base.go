// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/rintuphq/terraform-provider-rintup/internal/client"
)

// withClient is embedded by every resource and data source: it receives the configured API client.
type withClient struct {
	client *client.Client
}

func (w *withClient) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	w.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

type dataSourceWithClient struct {
	client *client.Client
}

func (w *dataSourceWithClient) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	w.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func clientFrom(data any, diags *diag.Diagnostics) *client.Client {
	if data == nil {
		return nil // provider not configured yet (validation phase)
	}
	c, ok := data.(*client.Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("expected *client.Client, got %T", data))
	}
	return c
}

// apiError reports a failed API call. The message carries status, catalogue code and field errors,
// never the request body (it may hold secrets).
func apiError(diags *diag.Diagnostics, action string, err error) {
	diags.AddError("RintUp API: "+action+" failed", err.Error())
}

// deleteIgnoringNotFound treats 404 as success: the object is already gone.
func deleteIgnoringNotFound(diags *diag.Diagnostics, action string, err error) {
	if err != nil && !client.IsNotFound(err) {
		apiError(diags, action, err)
	}
}
