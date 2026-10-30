// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

// Package client is a small, hand-written RintUp REST API client (docs/api/openapi.json).
//
// It covers only what the Terraform provider needs: JSON in and out, bearer authentication,
// cursor pagination ({ "data": [...], "nextCursor": ... }) and retries with exponential backoff
// on 429 (honouring Retry-After) and on 5xx for idempotent methods.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the public API root.
const DefaultBaseURL = "https://rintup.com/api/v1"

// Client talks to the RintUp API with an API key (Authorization: Bearer rup_live_...).
type Client struct {
	baseURL   string
	apiKey    string
	userAgent string
	http      *http.Client

	// MaxRetries is how many times a retryable request is repeated (default 5).
	MaxRetries int
	// BaseDelay is the first backoff delay; it doubles on every retry up to MaxDelay.
	BaseDelay time.Duration
	MaxDelay  time.Duration
	// Sleep waits between retries; tests replace it to avoid real waiting.
	Sleep func(ctx context.Context, d time.Duration) error
	// OnRequest, when set, is called after every response with method, path and status.
	// It never receives headers, bodies or query strings (they may contain secrets).
	OnRequest func(ctx context.Context, method, path string, status int)
}

// New builds a client. version ends up in the User-Agent: terraform-provider-rintup/<version>.
func New(baseURL, apiKey, version string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		userAgent:  "terraform-provider-rintup/" + version,
		http:       &http.Client{Timeout: 60 * time.Second},
		MaxRetries: 5,
		BaseDelay:  time.Second,
		MaxDelay:   30 * time.Second,
		Sleep:      sleepCtx,
	}
}

// APIError is a non-2xx answer. RintUp errors are application/problem+json with a catalogue code.
type APIError struct {
	Status int                 `json:"status"`
	Code   string              `json:"code"`
	Title  string              `json:"title"`
	Detail string              `json:"detail"`
	Errors map[string][]string `json:"errors"`
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "RintUp API %d", e.Status)
	if e.Code != "" {
		b.WriteString(" " + e.Code)
	}
	if e.Title != "" {
		b.WriteString(": " + e.Title)
	}
	if e.Detail != "" {
		b.WriteString(" (" + e.Detail + ")")
	}
	for field, msgs := range e.Errors {
		fmt.Fprintf(&b, "; %s: %s", field, strings.Join(msgs, ", "))
	}
	return b.String()
}

// IsNotFound reports a 404: the object was deleted (or belongs to another workspace).
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

// Do sends one JSON request and decodes the JSON answer into out (when out is not nil).
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, in, out any) error {
	var payload []byte
	if in != nil {
		var err error
		if payload, err = json.Marshal(in); err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
	}
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	for attempt := 0; ; attempt++ {
		resp, err := c.send(ctx, method, target, payload)
		if err != nil {
			if attempt < c.MaxRetries && idempotent(method) && ctx.Err() == nil {
				if werr := c.Sleep(ctx, c.backoff(attempt, "")); werr != nil {
					return werr
				}
				continue
			}
			return fmt.Errorf("%s %s: %w", method, path, err)
		}
		if c.OnRequest != nil {
			c.OnRequest(ctx, method, path, resp.StatusCode)
		}
		if attempt < c.MaxRetries && retryable(method, resp.StatusCode) {
			wait := c.backoff(attempt, resp.Header.Get("Retry-After"))
			drain(resp)
			if werr := c.Sleep(ctx, wait); werr != nil {
				return werr
			}
			continue
		}
		return decode(resp, out)
	}
}

func (c *Client) send(ctx context.Context, method, target string, payload []byte) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(req)
}

func decode(resp *http.Response, out any) error {
	defer drain(resp)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil || resp.StatusCode == http.StatusNoContent {
			return nil
		}
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("decode response: %w", err)
		}
		return nil
	}
	apiErr := &APIError{}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	_ = json.Unmarshal(raw, apiErr) // a non-JSON error body still yields a useful status
	apiErr.Status = resp.StatusCode
	if apiErr.Title == "" {
		apiErr.Title = http.StatusText(resp.StatusCode)
	}
	return apiErr
}

// retryable: 429 is always safe (the request was not processed). 5xx only for idempotent methods:
// the API has no Idempotency-Key yet, so a repeated POST could create a duplicate.
func retryable(method string, status int) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	switch status {
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return idempotent(method)
	}
	return false
}

func idempotent(method string) bool {
	return method != http.MethodPost
}

// backoff honours Retry-After (seconds or HTTP date), otherwise BaseDelay * 2^attempt, capped at MaxDelay.
func (c *Client) backoff(attempt int, retryAfter string) time.Duration {
	if retryAfter != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && secs >= 0 {
			return min(time.Duration(secs)*time.Second, 2*c.MaxDelay)
		}
		if at, err := http.ParseTime(retryAfter); err == nil {
			return min(max(time.Until(at), 0), 2*c.MaxDelay)
		}
	}
	d := c.BaseDelay << attempt
	if d <= 0 || d > c.MaxDelay {
		return c.MaxDelay
	}
	return d
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
