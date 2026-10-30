// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"net/http"
	"net/url"
)

// Mask is how the API shows a stored secret (header values, body, basic auth password).
// Sending it back (or omitting the field) on PATCH keeps the stored value; "" removes it.
const Mask = "••••"

// MonitorInput is the body of POST /monitors and PATCH /monitors/{id} (MonitorRequest / MonitorPatch).
// Nil fields are omitted: on POST the server default applies, on PATCH the field is unchanged.
type MonitorInput struct {
	Name                *string        `json:"name,omitempty"`
	Type                *string        `json:"type,omitempty"`
	IntervalSeconds     *int64         `json:"intervalSeconds,omitempty"`
	TimeoutMs           *int64         `json:"timeoutMs,omitempty"`
	Locations           *[]string      `json:"locations,omitempty"`
	Confirmations       *int64         `json:"confirmations,omitempty"`
	AggregateRule       *string        `json:"aggregateRule,omitempty"`
	TriggerDelaySeconds *int64         `json:"triggerDelaySeconds,omitempty"`
	RepeatEveryMinutes  *int64         `json:"repeatEveryMinutes,omitempty"`
	ContactGroupIDs     *[]string      `json:"contactGroupIds,omitempty"`
	Tags                *[]string      `json:"tags,omitempty"`
	HTTP                *HTTPSpec      `json:"http,omitempty"`
	TCP                 *TCPSpec       `json:"tcp,omitempty"`
	DNS                 *DNSSpec       `json:"dns,omitempty"`
	Ping                *PingSpec      `json:"ping,omitempty"`
	SSL                 *SSLSpec       `json:"ssl,omitempty"`
	Push                *PushSpec      `json:"push,omitempty"`
	Domain              *DomainSpec    `json:"domain,omitempty"`
	PageSpeed           *PageSpeedSpec `json:"pageSpeed,omitempty"`
}

// Monitor is MonitorDetail (GET/POST/PATCH answer). Secrets are masked.
type Monitor struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Type                string         `json:"type"`
	Target              string         `json:"target"`
	IntervalSeconds     int64          `json:"intervalSeconds"`
	TimeoutMs           int64          `json:"timeoutMs"`
	Locations           []string       `json:"locations"`
	Confirmations       int64          `json:"confirmations"`
	AggregateRule       string         `json:"aggregateRule"`
	TriggerDelaySeconds int64          `json:"triggerDelaySeconds"`
	RepeatEveryMinutes  int64          `json:"repeatEveryMinutes"`
	ContactGroupIDs     []string       `json:"contactGroupIds"`
	Tags                []string       `json:"tags"`
	Status              string         `json:"status"`
	Paused              bool           `json:"paused"`
	CreatedAt           string         `json:"createdAt"`
	LastCheckedAt       *string        `json:"lastCheckedAt"`
	HTTP                *HTTPSpec      `json:"http"`
	TCP                 *TCPSpec       `json:"tcp"`
	DNS                 *DNSSpec       `json:"dns"`
	Ping                *PingSpec      `json:"ping"`
	SSL                 *SSLSpec       `json:"ssl"`
	Push                *PushSpec      `json:"push"`
	PushURL             *string        `json:"pushUrl"`
	Domain              *DomainSpec    `json:"domain"`
	PageSpeed           *PageSpeedSpec `json:"pageSpeed"`
}

// MonitorListItem is one row of GET /monitors.
type MonitorListItem struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	Target          string   `json:"target"`
	Status          string   `json:"status"`
	IntervalSeconds int64    `json:"intervalSeconds"`
	Paused          bool     `json:"paused"`
	Locations       []string `json:"locations"`
	Tags            []string `json:"tags"`
}

// HTTPSpec is HttpInput. Headers values, Body and BasicAuthPassword are secrets.
type HTTPSpec struct {
	URL               *string            `json:"url,omitempty"`
	Method            *string            `json:"method,omitempty"`
	Headers           *map[string]string `json:"headers,omitempty"`
	Body              *string            `json:"body,omitempty"`
	ContentType       *string            `json:"contentType,omitempty"`
	BasicAuthUser     *string            `json:"basicAuthUser,omitempty"`
	BasicAuthPassword *string            `json:"basicAuthPassword,omitempty"`
	ExpectedStatus    *[]StatusRange     `json:"expectedStatus,omitempty"`
	KeywordPresent    *string            `json:"keywordPresent,omitempty"`
	KeywordAbsent     *string            `json:"keywordAbsent,omitempty"`
	FollowRedirects   *bool              `json:"followRedirects,omitempty"`
	UserAgent         *string            `json:"userAgent,omitempty"`
}

// StatusRange is an accepted HTTP status range (StatusRangeInput).
type StatusRange struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

// TCPSpec is TcpInput.
type TCPSpec struct {
	Host   *string `json:"host,omitempty"`
	Port   *int64  `json:"port,omitempty"`
	Send   *string `json:"send,omitempty"`
	Expect *string `json:"expect,omitempty"`
}

// DNSSpec is DnsInput.
type DNSSpec struct {
	Host       *string   `json:"host,omitempty"`
	RecordType *string   `json:"recordType,omitempty"`
	Expected   *[]string `json:"expected,omitempty"`
	Resolver   *string   `json:"resolver,omitempty"`
}

// PingSpec is PingInput.
type PingSpec struct {
	Host  *string `json:"host,omitempty"`
	Count *int64  `json:"count,omitempty"`
}

// SSLSpec is SslInput.
type SSLSpec struct {
	Host           *string  `json:"host,omitempty"`
	Port           *int64   `json:"port,omitempty"`
	ServerName     *string  `json:"serverName,omitempty"`
	ThresholdsDays *[]int64 `json:"thresholdsDays,omitempty"`
}

// PushSpec is PushInput (heartbeat monitor).
type PushSpec struct {
	PeriodSeconds *int64 `json:"periodSeconds,omitempty"`
	GraceSeconds  *int64 `json:"graceSeconds,omitempty"`
}

// DomainSpec is DomainInput (registration expiry and name servers).
type DomainSpec struct {
	Domain              *string   `json:"domain,omitempty"`
	ThresholdsDays      *[]int64  `json:"thresholdsDays,omitempty"`
	ExpectedNameservers *[]string `json:"expectedNameservers,omitempty"`
}

// PageSpeedSpec is PageSpeedInput.
type PageSpeedSpec struct {
	URL        *string `json:"url,omitempty"`
	MaxTotalMs *int64  `json:"maxTotalMs,omitempty"`
}

// CreateMonitor: POST /monitors.
func (c *Client) CreateMonitor(ctx context.Context, in *MonitorInput) (*Monitor, error) {
	var out Monitor
	return &out, c.Do(ctx, http.MethodPost, "/monitors", nil, in, &out)
}

// GetMonitor: GET /monitors/{id}.
func (c *Client) GetMonitor(ctx context.Context, id string) (*Monitor, error) {
	var out Monitor
	return &out, c.Do(ctx, http.MethodGet, "/monitors/"+url.PathEscape(id), nil, nil, &out)
}

// UpdateMonitor: PATCH /monitors/{id}.
func (c *Client) UpdateMonitor(ctx context.Context, id string, in *MonitorInput) (*Monitor, error) {
	var out Monitor
	return &out, c.Do(ctx, http.MethodPatch, "/monitors/"+url.PathEscape(id), nil, in, &out)
}

// DeleteMonitor: DELETE /monitors/{id}.
func (c *Client) DeleteMonitor(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/monitors/"+url.PathEscape(id), nil, nil, nil)
}

// SetMonitorPaused: POST /monitors/{id}/pause or /resume.
func (c *Client) SetMonitorPaused(ctx context.Context, id string, paused bool) error {
	action := "/resume"
	if paused {
		action = "/pause"
	}
	return c.Do(ctx, http.MethodPost, "/monitors/"+url.PathEscape(id)+action, nil, nil, nil)
}

// ListMonitors: GET /monitors (all pages). q filters by text in the name or target.
func (c *Client) ListMonitors(ctx context.Context, q string) ([]MonitorListItem, error) {
	query := url.Values{}
	if q != "" {
		query.Set("q", q)
	}
	return ListAll[MonitorListItem](ctx, c, "/monitors", query)
}
