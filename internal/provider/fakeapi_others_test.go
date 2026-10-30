// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"net/http"

	"github.com/rintuphq/terraform-provider-rintup/internal/client"
)

// contact groups and channels

func (f *fakeAPI) createGroup(w http.ResponseWriter, r *http.Request, _ string) {
	var in struct{ Name string }
	decodeBody(r, &in)
	g := &client.ContactGroup{ID: f.newID(), Name: in.Name, Channels: []client.Channel{}}
	f.groups[g.ID] = g
	writeJSON(w, http.StatusCreated, g)
}

func (f *fakeAPI) listGroups(w http.ResponseWriter, r *http.Request, _ string) {
	writeJSON(w, http.StatusOK, page(f, r, f.groups, nil))
}

func (f *fakeAPI) getGroup(w http.ResponseWriter, _ *http.Request, id string) {
	if g, ok := f.groups[id]; ok {
		writeJSON(w, http.StatusOK, g)
		return
	}
	notFound(w)
}

func (f *fakeAPI) patchGroup(w http.ResponseWriter, r *http.Request, id string) {
	g, ok := f.groups[id]
	if !ok {
		notFound(w)
		return
	}
	var in struct{ Name *string }
	decodeBody(r, &in)
	set(&g.Name, in.Name)
	writeJSON(w, http.StatusOK, g)
}

func (f *fakeAPI) deleteGroup(w http.ResponseWriter, _ *http.Request, id string) {
	if _, ok := f.groups[id]; !ok {
		notFound(w)
		return
	}
	delete(f.groups, id)
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeAPI) createChannel(w http.ResponseWriter, r *http.Request, groupID string) {
	g, ok := f.groups[groupID]
	if !ok {
		notFound(w)
		return
	}
	var in client.ChannelInput
	decodeBody(r, &in)
	ch := client.Channel{ID: f.newID(), Type: in.Type, Display: "***", RequiresVerification: in.Type == "email" || in.Type == "sms"}
	if in.Target != nil {
		f.channelSec[ch.ID+"/target"] = *in.Target
	}
	created := ch
	if in.Secret != nil {
		f.channelSec[ch.ID] = *in.Secret
	} else if in.Type == "webhook" {
		created.Secret = ptrTo("whsec_generated")
	}
	g.Channels = append(g.Channels, ch)
	writeJSON(w, http.StatusCreated, created)
}

func (f *fakeAPI) deleteChannel(w http.ResponseWriter, _ *http.Request, id string) {
	for _, g := range f.groups {
		for i, ch := range g.Channels {
			if ch.ID == id {
				g.Channels = append(g.Channels[:i], g.Channels[i+1:]...)
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
	}
	notFound(w)
}

// maintenance windows (no GET by id: like the real API)

func (f *fakeAPI) createWindow(w http.ResponseWriter, r *http.Request, _ string) {
	var in client.MaintenanceWindowInput
	decodeBody(r, &in)
	mw := &client.MaintenanceWindow{ID: f.newID(), Name: "Maintenance", StartsAt: "2026-10-30T10:00:00Z",
		EndsAt: ptrTo("2026-10-30T11:00:00Z"), MonitorIDs: []string{}, Recurrence: "none", Days: []int64{}}
	applyWindow(mw, &in, nil)
	if mw.Recurrence != "none" && in.EndsAt == nil {
		mw.EndsAt = nil // a recurring series without end
	}
	f.windows[mw.ID] = mw
	writeJSON(w, http.StatusCreated, mw)
}

func (f *fakeAPI) listWindows(w http.ResponseWriter, r *http.Request, _ string) {
	writeJSON(w, http.StatusOK, page(f, r, f.windows, nil))
}

func (f *fakeAPI) patchWindow(w http.ResponseWriter, r *http.Request, id string) {
	mw, ok := f.windows[id]
	if !ok {
		notFound(w)
		return
	}
	var raw map[string]json.RawMessage
	decodeBody(r, &raw)
	body, _ := json.Marshal(raw)
	var in client.MaintenanceWindowInput
	_ = json.Unmarshal(body, &in)
	applyWindow(mw, &in, raw)
	writeJSON(w, http.StatusOK, mw)
}

func applyWindow(mw *client.MaintenanceWindow, in *client.MaintenanceWindowInput, raw map[string]json.RawMessage) {
	set(&mw.Name, in.Name)
	set(&mw.StartsAt, in.StartsAt)
	setPtr(&mw.EndsAt, in.EndsAt)
	if v, ok := raw["endsAt"]; ok && string(v) == "null" {
		mw.EndsAt = nil
	}
	set(&mw.MonitorIDs, in.MonitorIDs)
	set(&mw.Recurrence, in.Recurrence)
	set(&mw.Days, in.Days)
	setPtr(&mw.StartTime, in.StartTime)
	setPtr(&mw.DurationMinutes, in.DurationMinutes)
	setPtr(&mw.TimeZone, in.TimeZone)
	if mw.Recurrence != "none" {
		mw.DurationMinutes = or(mw.DurationMinutes, 60)
		mw.TimeZone = or(mw.TimeZone, "Europe/Istanbul")
	}
}

func (f *fakeAPI) deleteWindow(w http.ResponseWriter, _ *http.Request, id string) {
	if _, ok := f.windows[id]; !ok {
		notFound(w)
		return
	}
	delete(f.windows, id)
	w.WriteHeader(http.StatusOK)
}

// status pages

func (f *fakeAPI) createPage(w http.ResponseWriter, r *http.Request, _ string) {
	var in client.StatusPageInput
	decodeBody(r, &in)
	sp := &client.StatusPage{ID: f.newID(), Slug: "acme-status", ShowResponseTimes: true, Locale: "en", Components: []client.Component{}}
	f.applyPage(sp, &in)
	f.pages[sp.ID] = sp
	writeJSON(w, http.StatusCreated, sp)
}

func (f *fakeAPI) getPage(w http.ResponseWriter, _ *http.Request, id string) {
	if sp, ok := f.pages[id]; ok {
		writeJSON(w, http.StatusOK, sp)
		return
	}
	notFound(w)
}

func (f *fakeAPI) patchPage(w http.ResponseWriter, r *http.Request, id string) {
	sp, ok := f.pages[id]
	if !ok {
		notFound(w)
		return
	}
	var in client.StatusPageInput
	decodeBody(r, &in)
	f.applyPage(sp, &in)
	writeJSON(w, http.StatusOK, sp)
}

func (f *fakeAPI) applyPage(sp *client.StatusPage, in *client.StatusPageInput) {
	set(&sp.Title, in.Title)
	set(&sp.Slug, in.Slug)
	setOptional(&sp.Description, in.Description)
	setOptional(&sp.LogoURL, in.LogoURL)
	setOptional(&sp.BrandColor, in.BrandColor)
	setOptional(&sp.CustomDomain, in.CustomDomain)
	set(&sp.ShowResponseTimes, in.ShowResponseTimes)
	set(&sp.Noindex, in.Noindex)
	set(&sp.Locale, in.Locale)
	set(&sp.AllowSubscriptions, in.AllowSubscriptions)
	set(&sp.NotifySubscribersOnOutage, in.NotifySubscribersOnOutage)
	if in.Components != nil {
		comps := []client.Component{}
		for i, c := range *in.Components {
			name := c.DisplayName
			if name == nil {
				if m, ok := f.monitors[c.MonitorID]; ok {
					name = ptrTo(m.Name)
				}
			}
			comps = append(comps, client.Component{ID: "c-" + c.MonitorID, MonitorID: c.MonitorID, DisplayName: name, Group: c.Group, Order: int64(i)})
		}
		sp.Components = comps
	}
}

func (f *fakeAPI) deletePage(w http.ResponseWriter, _ *http.Request, id string) {
	if _, ok := f.pages[id]; !ok {
		notFound(w)
		return
	}
	delete(f.pages, id)
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeAPI) setPassword(w http.ResponseWriter, r *http.Request, id string) {
	sp, ok := f.pages[id]
	if !ok {
		notFound(w)
		return
	}
	var in struct{ Password string }
	decodeBody(r, &in)
	f.passwords[id] = in.Password
	sp.PasswordProtected = true
	w.WriteHeader(http.StatusOK)
}

func (f *fakeAPI) removePassword(w http.ResponseWriter, _ *http.Request, id string) {
	sp, ok := f.pages[id]
	if !ok {
		notFound(w)
		return
	}
	delete(f.passwords, id)
	sp.PasswordProtected = false
	w.WriteHeader(http.StatusOK)
}
