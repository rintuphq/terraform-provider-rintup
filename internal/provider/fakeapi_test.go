// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/rintuphq/terraform-provider-rintup/internal/client"
)

// fakeAPI is a small in-memory RintUp API: enough of the real behaviour (defaults filled on create,
// PATCH merge, masked secrets, cursor pages, 404 for missing objects, 429 + Retry-After) to drive the
// provider through Terraform end to end without a network.
type fakeAPI struct {
	t   *testing.T
	srv *httptest.Server
	mu  sync.Mutex
	seq int

	pageSize   int
	throttle   int // the next N requests answer 429 with Retry-After: 0
	throttled  int
	monitors   map[string]*client.Monitor
	secrets    map[string]*httpSecrets // monitor id -> real secret values
	groups     map[string]*client.ContactGroup
	channelSec map[string]string // channel id -> secret / secret target received
	windows    map[string]*client.MaintenanceWindow
	pages      map[string]*client.StatusPage
	passwords  map[string]string
	order      []string // creation order of every id, for stable listing
}

type httpSecrets struct {
	Headers  map[string]string
	Body     string
	Password string
}

func newFakeAPI(t *testing.T) *fakeAPI {
	f := &fakeAPI{t: t, pageSize: 2,
		monitors: map[string]*client.Monitor{}, secrets: map[string]*httpSecrets{}, groups: map[string]*client.ContactGroup{},
		channelSec: map[string]string{}, windows: map[string]*client.MaintenanceWindow{}, pages: map[string]*client.StatusPage{},
		passwords: map[string]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) providerConfig() string {
	return fmt.Sprintf("provider \"rintup\" {\n  api_key  = \"rup_live_test\"\n  base_url = %q\n}\n", f.srv.URL+"/api/v1")
}

func (f *fakeAPI) newID() string {
	f.seq++
	id := fmt.Sprintf("00000000-0000-7000-8000-%012d", f.seq)
	f.order = append(f.order, id)
	return id
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer rup_live_test" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if f.throttle > 0 {
		f.throttle--
		f.throttled++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/")
	route := r.Method + " " + parts[0]
	if len(parts) > 1 {
		route += "/{id}"
	}
	if len(parts) > 2 {
		route += "/" + parts[2]
	}
	id := ""
	if len(parts) > 1 {
		id = parts[1]
	}
	handlers := map[string]func(http.ResponseWriter, *http.Request, string){
		"POST monitors": f.createMonitor, "GET monitors": f.listMonitors, "GET monitors/{id}": f.getMonitor,
		"PATCH monitors/{id}": f.patchMonitor, "DELETE monitors/{id}": f.deleteMonitor,
		"POST monitors/{id}/pause": f.pause(true), "POST monitors/{id}/resume": f.pause(false),
		"POST contact-groups": f.createGroup, "GET contact-groups": f.listGroups, "GET contact-groups/{id}": f.getGroup,
		"PATCH contact-groups/{id}": f.patchGroup, "DELETE contact-groups/{id}": f.deleteGroup,
		"POST contact-groups/{id}/channels": f.createChannel, "DELETE channels/{id}": f.deleteChannel,
		"POST maintenance-windows": f.createWindow, "GET maintenance-windows": f.listWindows,
		"PATCH maintenance-windows/{id}": f.patchWindow, "DELETE maintenance-windows/{id}": f.deleteWindow,
		"POST status-pages": f.createPage, "GET status-pages/{id}": f.getPage, "PATCH status-pages/{id}": f.patchPage,
		"DELETE status-pages/{id}": f.deletePage, "PUT status-pages/{id}/password": f.setPassword,
		"DELETE status-pages/{id}/password": f.removePassword, "GET locations": f.locations,
	}
	h, ok := handlers[route]
	if !ok {
		f.t.Errorf("fake API: unexpected %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
	h(w, r, id)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func notFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, map[string]any{"status": 404, "title": "Not found", "code": "RUP-PLT-404-NOT_FOUND"})
}

func decodeBody(r *http.Request, v any) {
	_ = json.NewDecoder(r.Body).Decode(v)
}

// page cuts ids (in creation order) into cursor pages of f.pageSize, like the real keyset pages.
func page[T any](f *fakeAPI, r *http.Request, all map[string]*T, keep func(*T) bool) map[string]any {
	ids := []string{}
	for _, id := range f.order {
		if v, ok := all[id]; ok && (keep == nil || keep(v)) {
			ids = append(ids, id)
		}
	}
	start, _ := strconv.Atoi(r.URL.Query().Get("cursor"))
	end := min(start+f.pageSize, len(ids))
	data := []*T{}
	for _, id := range ids[start:end] {
		data = append(data, all[id])
	}
	var next any
	if end < len(ids) {
		next = strconv.Itoa(end)
	}
	return map[string]any{"data": data, "nextCursor": next}
}

func (f *fakeAPI) locations(w http.ResponseWriter, _ *http.Request, _ string) {
	writeJSON(w, http.StatusOK, map[string]any{"nextCursor": nil, "data": []client.Location{
		{Code: "fra", Name: "Frankfurt", Country: "DE", Online: true, IPv4: []string{"192.0.2.10"}, IPv6: []string{"2001:db8::10"}},
		{Code: "ist", Name: "Istanbul", Country: "TR", Online: true, IPv4: []string{"192.0.2.20"}, IPv6: []string{}},
	}})
}

// helpers shared by the fake handlers

func ptrTo[T any](v T) *T { return &v }

func or[T any](p *T, def T) *T {
	if p == nil {
		return &def
	}
	return p
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
