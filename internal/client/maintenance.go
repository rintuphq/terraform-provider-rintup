// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"net/http"
	"net/url"
)

// MaintenanceWindow is MaintenanceWindowView.
type MaintenanceWindow struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	StartsAt        string   `json:"startsAt"`
	EndsAt          *string  `json:"endsAt"`
	MonitorIDs      []string `json:"monitorIds"`
	Active          bool     `json:"active"`
	Recurrence      string   `json:"recurrence"`
	Days            []int64  `json:"days"`
	StartTime       *string  `json:"startTime"`
	DurationMinutes *int64   `json:"durationMinutes"`
	TimeZone        *string  `json:"timeZone"`
	NextStartsAt    *string  `json:"nextStartsAt"`
	NextEndsAt      *string  `json:"nextEndsAt"`
}

// MaintenanceWindowInput is MaintenanceWindowRequest / MaintenanceWindowChanges.
// EndsAt is special on PATCH: an explicit null clears it (a recurring series without end); set
// ClearEndsAt to send that null.
type MaintenanceWindowInput struct {
	Name            *string   `json:"name,omitempty"`
	StartsAt        *string   `json:"startsAt,omitempty"`
	EndsAt          *string   `json:"endsAt,omitempty"`
	MonitorIDs      *[]string `json:"monitorIds,omitempty"`
	Recurrence      *string   `json:"recurrence,omitempty"`
	Days            *[]int64  `json:"days,omitempty"`
	StartTime       *string   `json:"startTime,omitempty"`
	DurationMinutes *int64    `json:"durationMinutes,omitempty"`
	TimeZone        *string   `json:"timeZone,omitempty"`
	// ClearEndsAt sends "endsAt": null (PATCH only).
	ClearEndsAt bool `json:"-"`
}

func (in *MaintenanceWindowInput) body() any {
	if !in.ClearEndsAt {
		return in
	}
	type plain MaintenanceWindowInput
	return struct {
		*plain
		EndsAt *string `json:"endsAt"`
	}{plain: (*plain)(in)}
}

// CreateMaintenanceWindow: POST /maintenance-windows.
func (c *Client) CreateMaintenanceWindow(ctx context.Context, in *MaintenanceWindowInput) (*MaintenanceWindow, error) {
	var out MaintenanceWindow
	return &out, c.Do(ctx, http.MethodPost, "/maintenance-windows", nil, in, &out)
}

// GetMaintenanceWindow finds one window by id. The API has no GET /maintenance-windows/{id},
// so the (paged) list is scanned; absence is a 404 APIError like on the other resources.
func (c *Client) GetMaintenanceWindow(ctx context.Context, id string) (*MaintenanceWindow, error) {
	w, err := Find(ctx, c, "/maintenance-windows", nil, func(w MaintenanceWindow) bool { return w.ID == id })
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, &APIError{Status: http.StatusNotFound, Title: "maintenance window not found"}
	}
	return w, nil
}

// UpdateMaintenanceWindow: PATCH /maintenance-windows/{id}.
func (c *Client) UpdateMaintenanceWindow(ctx context.Context, id string, in *MaintenanceWindowInput) (*MaintenanceWindow, error) {
	var out MaintenanceWindow
	return &out, c.Do(ctx, http.MethodPatch, "/maintenance-windows/"+url.PathEscape(id), nil, in.body(), &out)
}

// DeleteMaintenanceWindow: DELETE /maintenance-windows/{id}.
func (c *Client) DeleteMaintenanceWindow(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/maintenance-windows/"+url.PathEscape(id), nil, nil, nil)
}

// Location is LocationView (fixed egress addresses customers allow-list).
type Location struct {
	Code    string   `json:"code"`
	Name    string   `json:"name"`
	Country string   `json:"country"`
	Online  bool     `json:"online"`
	IPv4    []string `json:"ipv4"`
	IPv6    []string `json:"ipv6"`
}

// ListLocations: GET /locations (a single, small page; takes no limit/cursor).
func (c *Client) ListLocations(ctx context.Context) ([]Location, error) {
	var page Page[Location]
	err := c.Do(ctx, http.MethodGet, "/locations", nil, nil, &page)
	return page.Data, err
}
