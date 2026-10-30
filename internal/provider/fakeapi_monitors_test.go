// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/rintuphq/terraform-provider-rintup/internal/client"
)

func (f *fakeAPI) createMonitor(w http.ResponseWriter, r *http.Request, _ string) {
	var in client.MonitorInput
	decodeBody(r, &in)
	if in.Type == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": 400, "title": "Validation failed", "errors": map[string][]string{"type": {"required"}}})
		return
	}
	id := f.newID()
	m := &client.Monitor{ID: id, Type: *in.Type, IntervalSeconds: 300, TimeoutMs: 10000, Locations: []string{"fra", "ist"},
		Confirmations: 2, AggregateRule: "majority", ContactGroupIDs: []string{}, Tags: []string{}, Status: "pending",
		CreatedAt: "2026-10-30T10:00:00Z"}
	f.secrets[id] = &httpSecrets{Headers: map[string]string{}}
	switch m.Type {
	case "http":
		m.HTTP = &client.HTTPSpec{Method: ptrTo("GET"), FollowRedirects: ptrTo(true), ExpectedStatus: &[]client.StatusRange{}}
	case "tcp":
		m.TCP = &client.TCPSpec{}
	case "push":
		m.Push = &client.PushSpec{}
		m.PushURL = ptrTo("/api/v1/push/tok-" + id)
	}
	f.applyMonitor(m, &in)
	if m.Name == "" {
		m.Name = m.Target
	}
	f.monitors[id] = m
	writeJSON(w, http.StatusCreated, f.masked(m))
}

func (f *fakeAPI) getMonitor(w http.ResponseWriter, _ *http.Request, id string) {
	if m, ok := f.monitors[id]; ok {
		writeJSON(w, http.StatusOK, f.masked(m))
		return
	}
	notFound(w)
}

func (f *fakeAPI) patchMonitor(w http.ResponseWriter, r *http.Request, id string) {
	m, ok := f.monitors[id]
	if !ok {
		notFound(w)
		return
	}
	var in client.MonitorInput
	decodeBody(r, &in)
	f.applyMonitor(m, &in)
	writeJSON(w, http.StatusOK, f.masked(m))
}

func (f *fakeAPI) deleteMonitor(w http.ResponseWriter, _ *http.Request, id string) {
	if _, ok := f.monitors[id]; !ok {
		notFound(w)
		return
	}
	delete(f.monitors, id)
	w.WriteHeader(http.StatusOK)
}

func (f *fakeAPI) pause(paused bool) func(http.ResponseWriter, *http.Request, string) {
	return func(w http.ResponseWriter, _ *http.Request, id string) {
		m, ok := f.monitors[id]
		if !ok {
			notFound(w)
			return
		}
		m.Paused = paused
		w.WriteHeader(http.StatusOK)
	}
}

func (f *fakeAPI) listMonitors(w http.ResponseWriter, r *http.Request, _ string) {
	q := r.URL.Query().Get("q")
	resp := page(f, r, f.monitors, func(m *client.Monitor) bool { return q == "" || strings.Contains(m.Name+m.Target, q) })
	items := []client.MonitorListItem{}
	for _, m := range resp["data"].([]*client.Monitor) {
		items = append(items, client.MonitorListItem{ID: m.ID, Name: m.Name, Type: m.Type, Target: m.Target, Status: m.Status,
			IntervalSeconds: m.IntervalSeconds, Paused: m.Paused, Locations: m.Locations, Tags: m.Tags})
	}
	resp["data"] = items
	writeJSON(w, http.StatusOK, resp)
}

// applyMonitor merges like the server: given fields replace, type blocks merge field by field,
// "" clears optional text, and a secret that is null or the mask keeps its stored value.
func (f *fakeAPI) applyMonitor(m *client.Monitor, in *client.MonitorInput) {
	set(&m.Name, in.Name)
	set(&m.IntervalSeconds, in.IntervalSeconds)
	set(&m.TimeoutMs, in.TimeoutMs)
	set(&m.Locations, in.Locations)
	set(&m.Confirmations, in.Confirmations)
	set(&m.AggregateRule, in.AggregateRule)
	set(&m.TriggerDelaySeconds, in.TriggerDelaySeconds)
	set(&m.RepeatEveryMinutes, in.RepeatEveryMinutes)
	set(&m.ContactGroupIDs, in.ContactGroupIDs)
	set(&m.Tags, in.Tags)
	if h := in.HTTP; h != nil && m.HTTP != nil {
		f.applyHTTP(m.ID, m.HTTP, h)
		m.Target = *m.HTTP.URL
		if m.Name == "" {
			u, _ := url.Parse(*m.HTTP.URL)
			m.Target, m.Name = *m.HTTP.URL, u.Host
		}
	}
	if t := in.TCP; t != nil && m.TCP != nil {
		setPtr(&m.TCP.Host, t.Host)
		setPtr(&m.TCP.Port, t.Port)
		setOptional(&m.TCP.Send, t.Send)
		setOptional(&m.TCP.Expect, t.Expect)
		m.Target = fmt.Sprintf("%s:%d", *m.TCP.Host, *m.TCP.Port)
	}
	if m.Push != nil {
		if p := in.Push; p != nil {
			setPtr(&m.Push.PeriodSeconds, p.PeriodSeconds)
			setPtr(&m.Push.GraceSeconds, p.GraceSeconds)
		}
		m.Push.PeriodSeconds = or(m.Push.PeriodSeconds, m.IntervalSeconds)
		m.Push.GraceSeconds = or(m.Push.GraceSeconds, 60)
		m.Target = "Heartbeat"
	}
}

func (f *fakeAPI) applyHTTP(id string, cur, in *client.HTTPSpec) {
	setPtr(&cur.URL, in.URL)
	setPtr(&cur.Method, in.Method)
	setPtr(&cur.FollowRedirects, in.FollowRedirects)
	setPtr(&cur.ExpectedStatus, in.ExpectedStatus)
	for _, p := range []struct{ dst, src **string }{{&cur.ContentType, &in.ContentType}, {&cur.BasicAuthUser, &in.BasicAuthUser},
		{&cur.KeywordPresent, &in.KeywordPresent}, {&cur.KeywordAbsent, &in.KeywordAbsent}, {&cur.UserAgent, &in.UserAgent}} {
		setOptional(p.dst, *p.src)
	}
	sec := f.secrets[id]
	if in.Headers != nil {
		next := map[string]string{}
		for k, v := range *in.Headers {
			if v == client.Mask {
				v = sec.Headers[k]
			}
			next[k] = v
		}
		sec.Headers = next
	}
	secret(&sec.Body, in.Body)
	secret(&sec.Password, in.BasicAuthPassword)
}

// masked is the API view: secrets replaced by the mask.
func (f *fakeAPI) masked(m *client.Monitor) client.Monitor {
	out := *m
	if m.HTTP != nil {
		h := *m.HTTP
		sec := f.secrets[m.ID]
		headers := map[string]string{}
		for _, k := range sortedKeys(sec.Headers) {
			headers[k] = client.Mask
		}
		h.Headers = &headers
		if sec.Body != "" {
			h.Body = ptrTo(client.Mask)
		}
		if sec.Password != "" {
			h.BasicAuthPassword = ptrTo(client.Mask)
		}
		out.HTTP = &h
	}
	return out
}

func set[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

func setPtr[T any](dst **T, src *T) {
	if src != nil {
		v := *src
		*dst = &v
	}
}

func setOptional(dst **string, src *string) {
	switch {
	case src == nil:
	case *src == "":
		*dst = nil
	default:
		v := *src
		*dst = &v
	}
}

func secret(dst *string, src *string) {
	if src != nil && *src != client.Mask {
		*dst = *src
	}
}
