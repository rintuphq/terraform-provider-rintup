// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"net/http"
	"net/url"
)

// ContactGroup is ContactGroupResponse.
type ContactGroup struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Channels []Channel `json:"channels"`
}

// Channel is ChannelResponse: the target is masked (display) and the secret never comes back.
type Channel struct {
	ID                   string  `json:"id"`
	Type                 string  `json:"type"`
	Display              string  `json:"display"`
	Verified             bool    `json:"verified"`
	RequiresVerification bool    `json:"requiresVerification"`
	Secret               *string `json:"secret,omitempty"` // only in the create answer, only when generated
}

// ChannelInput is ChannelRequest.
type ChannelInput struct {
	Type   string  `json:"type"`
	Target *string `json:"target,omitempty"`
	Secret *string `json:"secret,omitempty"`
	Locale *string `json:"locale,omitempty"`
}

type contactGroupInput struct {
	Name string `json:"name"`
}

// CreateContactGroup: POST /contact-groups.
func (c *Client) CreateContactGroup(ctx context.Context, name string) (*ContactGroup, error) {
	var out ContactGroup
	return &out, c.Do(ctx, http.MethodPost, "/contact-groups", nil, contactGroupInput{Name: name}, &out)
}

// GetContactGroup: GET /contact-groups/{id}.
func (c *Client) GetContactGroup(ctx context.Context, id string) (*ContactGroup, error) {
	var out ContactGroup
	return &out, c.Do(ctx, http.MethodGet, "/contact-groups/"+url.PathEscape(id), nil, nil, &out)
}

// UpdateContactGroup: PATCH /contact-groups/{id}.
func (c *Client) UpdateContactGroup(ctx context.Context, id, name string) (*ContactGroup, error) {
	var out ContactGroup
	return &out, c.Do(ctx, http.MethodPatch, "/contact-groups/"+url.PathEscape(id), nil, contactGroupInput{Name: name}, &out)
}

// DeleteContactGroup: DELETE /contact-groups/{id} (removes its channels too).
func (c *Client) DeleteContactGroup(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/contact-groups/"+url.PathEscape(id), nil, nil, nil)
}

// ListContactGroups: GET /contact-groups (all pages).
func (c *Client) ListContactGroups(ctx context.Context) ([]ContactGroup, error) {
	return ListAll[ContactGroup](ctx, c, "/contact-groups", nil)
}

// CreateChannel: POST /contact-groups/{groupId}/channels.
func (c *Client) CreateChannel(ctx context.Context, groupID string, in *ChannelInput) (*Channel, error) {
	var out Channel
	return &out, c.Do(ctx, http.MethodPost, "/contact-groups/"+url.PathEscape(groupID)+"/channels", nil, in, &out)
}

// DeleteChannel: DELETE /channels/{id}.
func (c *Client) DeleteChannel(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/channels/"+url.PathEscape(id), nil, nil, nil)
}

// FindChannel looks a channel up. The API has no GET /channels/{id}: with a group id the group is read,
// otherwise every group is scanned. It returns the channel and its group id, or a 404 APIError.
func (c *Client) FindChannel(ctx context.Context, groupID, channelID string) (*Channel, string, error) {
	var groups []ContactGroup
	if groupID != "" {
		g, err := c.GetContactGroup(ctx, groupID)
		if err != nil {
			return nil, "", err
		}
		groups = []ContactGroup{*g}
	} else {
		var err error
		if groups, err = c.ListContactGroups(ctx); err != nil {
			return nil, "", err
		}
	}
	for _, g := range groups {
		for i := range g.Channels {
			if g.Channels[i].ID == channelID {
				return &g.Channels[i], g.ID, nil
			}
		}
	}
	return nil, "", &APIError{Status: http.StatusNotFound, Title: "channel not found"}
}
