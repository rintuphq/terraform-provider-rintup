// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"net/http"
	"net/url"
)

// StatusPage is StatusPageView.
type StatusPage struct {
	ID                        string      `json:"id"`
	Title                     string      `json:"title"`
	Slug                      string      `json:"slug"`
	Description               *string     `json:"description"`
	LogoURL                   *string     `json:"logoUrl"`
	BrandColor                *string     `json:"brandColor"`
	ShowResponseTimes         bool        `json:"showResponseTimes"`
	Noindex                   bool        `json:"noindex"`
	Locale                    string      `json:"locale"`
	CustomDomain              *string     `json:"customDomain"`
	Components                []Component `json:"components"`
	CustomDomainVerified      bool        `json:"customDomainVerified"`
	AllowSubscriptions        bool        `json:"allowSubscriptions"`
	NotifySubscribersOnOutage bool        `json:"notifySubscribersOnOutage"`
	PasswordProtected         bool        `json:"passwordProtected"`
}

// Component is ComponentView (answer) and ComponentInput (request; ID and Order are not sent).
type Component struct {
	ID          string  `json:"id,omitempty"`
	MonitorID   string  `json:"monitorId"`
	DisplayName *string `json:"displayName,omitempty"`
	Group       *string `json:"group,omitempty"`
	Order       int64   `json:"order,omitempty"`
}

// StatusPageInput is StatusPageInput (POST) and StatusPagePatch (PATCH).
// On PATCH "" clears description, logoUrl, brandColor and customDomain; components replace the whole list.
type StatusPageInput struct {
	Title                     *string      `json:"title,omitempty"`
	Slug                      *string      `json:"slug,omitempty"`
	Description               *string      `json:"description,omitempty"`
	LogoURL                   *string      `json:"logoUrl,omitempty"`
	BrandColor                *string      `json:"brandColor,omitempty"`
	ShowResponseTimes         *bool        `json:"showResponseTimes,omitempty"`
	Noindex                   *bool        `json:"noindex,omitempty"`
	Locale                    *string      `json:"locale,omitempty"`
	CustomDomain              *string      `json:"customDomain,omitempty"`
	Components                *[]Component `json:"components,omitempty"`
	AllowSubscriptions        *bool        `json:"allowSubscriptions,omitempty"`
	NotifySubscribersOnOutage *bool        `json:"notifySubscribersOnOutage,omitempty"`
}

func statusPagePath(id string) string { return "/status-pages/" + url.PathEscape(id) }

// CreateStatusPage: POST /status-pages.
func (c *Client) CreateStatusPage(ctx context.Context, in *StatusPageInput) (*StatusPage, error) {
	var out StatusPage
	return &out, c.Do(ctx, http.MethodPost, "/status-pages", nil, in, &out)
}

// GetStatusPage: GET /status-pages/{id}.
func (c *Client) GetStatusPage(ctx context.Context, id string) (*StatusPage, error) {
	var out StatusPage
	return &out, c.Do(ctx, http.MethodGet, statusPagePath(id), nil, nil, &out)
}

// UpdateStatusPage: PATCH /status-pages/{id}.
func (c *Client) UpdateStatusPage(ctx context.Context, id string, in *StatusPageInput) (*StatusPage, error) {
	var out StatusPage
	return &out, c.Do(ctx, http.MethodPatch, statusPagePath(id), nil, in, &out)
}

// DeleteStatusPage: DELETE /status-pages/{id}.
func (c *Client) DeleteStatusPage(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, statusPagePath(id), nil, nil, nil)
}

// SetStatusPagePassword: PUT /status-pages/{id}/password (protects the page or changes the password).
func (c *Client) SetStatusPagePassword(ctx context.Context, id, password string) error {
	body := struct {
		Password string `json:"password"`
	}{password}
	return c.Do(ctx, http.MethodPut, statusPagePath(id)+"/password", nil, body, nil)
}

// RemoveStatusPagePassword: DELETE /status-pages/{id}/password.
func (c *Client) RemoveStatusPagePassword(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, statusPagePath(id)+"/password", nil, nil, nil)
}
