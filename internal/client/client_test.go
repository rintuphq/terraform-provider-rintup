// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient returns a client against srv whose waits are recorded instead of slept.
func newTestClient(t *testing.T, srv *httptest.Server) (*Client, *[]time.Duration) {
	t.Helper()
	c := New(srv.URL+"/api/v1", "rup_live_test", "1.2.3")
	var waits []time.Duration
	c.Sleep = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}
	return c, &waits
}

func TestHeadersAndProblemDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer rup_live_test" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "terraform-provider-rintup/1.2.3" {
			t.Errorf("User-Agent = %q", got)
		}
		if r.URL.Path != "/api/v1/monitors" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":400,"title":"Validation failed","code":"RUP-PLT-400-VALIDATION_FAILED","errors":{"http.url":["required"]}}`))
	}))
	defer srv.Close()
	c, _ := newTestClient(t, srv)

	_, err := c.CreateMonitor(context.Background(), &MonitorInput{Type: ptr("http")})
	if err == nil || !strings.Contains(err.Error(), "RUP-PLT-400-VALIDATION_FAILED") || !strings.Contains(err.Error(), "http.url: required") {
		t.Fatalf("error = %v", err)
	}
	if IsNotFound(err) {
		t.Fatal("400 is not a 404")
	}
}

func TestRetries429HonouringRetryAfter(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) <= 2 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(Monitor{ID: "m1", Name: "api"})
	}))
	defer srv.Close()
	c, waits := newTestClient(t, srv)

	// POST is retried on 429: the server did not process the request.
	m, err := c.CreateMonitor(context.Background(), &MonitorInput{Type: ptr("http")})
	if err != nil || m.ID != "m1" {
		t.Fatalf("monitor = %+v, err = %v", m, err)
	}
	if calls.Load() != 3 || len(*waits) != 2 || (*waits)[0] != 7*time.Second {
		t.Fatalf("calls = %d, waits = %v", calls.Load(), *waits)
	}
}

func TestRetries5xxWithExponentialBackoffOnlyWhenIdempotent(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c, waits := newTestClient(t, srv)
	c.MaxRetries = 4

	if _, err := c.GetMonitor(context.Background(), "m1"); err == nil {
		t.Fatal("expected an error after retries")
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	if calls.Load() != 5 || fmt.Sprint(*waits) != fmt.Sprint(want) {
		t.Fatalf("GET: calls = %d, waits = %v", calls.Load(), *waits)
	}

	calls.Store(0)
	*waits = nil
	if _, err := c.CreateMonitor(context.Background(), &MonitorInput{}); err == nil {
		t.Fatal("expected an error")
	}
	if calls.Load() != 1 || len(*waits) != 0 {
		t.Fatalf("POST must not be retried on 5xx (no idempotency key): calls = %d", calls.Load())
	}
}

func TestBackoffIsCapped(t *testing.T) {
	c := New("", "", "dev")
	if d := c.backoff(20, ""); d != c.MaxDelay {
		t.Fatalf("backoff = %v", d)
	}
	if d := c.backoff(0, "3600"); d != 2*c.MaxDelay {
		t.Fatalf("Retry-After cap = %v", d)
	}
}

func TestNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c, _ := newTestClient(t, srv)
	if _, err := c.GetStatusPage(context.Background(), "x"); !IsNotFound(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestPaginationMergesAllPages(t *testing.T) {
	var seenCursors []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "200" || r.URL.Query().Get("q") != "api" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		cursor := r.URL.Query().Get("cursor")
		seenCursors = append(seenCursors, cursor)
		next := map[string]any{"": "c1", "c1": "c2", "c2": nil}[cursor]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       []MonitorListItem{{ID: "id-" + cursor}},
			"nextCursor": next,
		})
	}))
	defer srv.Close()
	c, _ := newTestClient(t, srv)

	items, err := c.ListMonitors(context.Background(), "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].ID != "id-" || items[2].ID != "id-c2" {
		t.Fatalf("items = %+v", items)
	}
	if fmt.Sprint(seenCursors) != "[ c1 c2]" {
		t.Fatalf("cursors = %v", seenCursors)
	}
}

func TestMaintenanceWindowLookupScansPagesAndClearsEndsAt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if v, ok := body["endsAt"]; !ok || v != nil {
				t.Errorf("PATCH body must carry endsAt: null, got %v", body)
			}
			_ = json.NewEncoder(w).Encode(MaintenanceWindow{ID: "w2"})
			return
		}
		if r.URL.Query().Get("cursor") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []MaintenanceWindow{{ID: "w1"}}, "nextCursor": "p2"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []MaintenanceWindow{{ID: "w2", Name: "db"}}, "nextCursor": nil})
	}))
	defer srv.Close()
	c, _ := newTestClient(t, srv)

	w, err := c.GetMaintenanceWindow(context.Background(), "w2")
	if err != nil || w.Name != "db" {
		t.Fatalf("w = %+v, err = %v", w, err)
	}
	if _, err := c.GetMaintenanceWindow(context.Background(), "nope"); !IsNotFound(err) {
		t.Fatalf("missing window must be a 404, got %v", err)
	}
	if _, err := c.UpdateMaintenanceWindow(context.Background(), "w2", &MaintenanceWindowInput{ClearEndsAt: true}); err != nil {
		t.Fatal(err)
	}
}

func ptr[T any](v T) *T { return &v }
