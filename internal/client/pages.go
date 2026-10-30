// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// PageLimit is the page size the client asks for (the API allows 1..200).
const PageLimit = 200

// Page is the envelope of every list endpoint: { "data": [...], "nextCursor": "..." | null }.
type Page[T any] struct {
	Data       []T     `json:"data"`
	NextCursor *string `json:"nextCursor"`
}

// EachPage walks a list endpoint page by page; visit returns true to stop early.
// Small lists (locations) are a single page whose nextCursor is always null.
func EachPage[T any](ctx context.Context, c *Client, path string, query url.Values, visit func([]T) bool) error {
	q := url.Values{}
	for k, v := range query {
		q[k] = v
	}
	q.Set("limit", strconv.Itoa(PageLimit))
	seen := map[string]bool{}
	for {
		var page Page[T]
		if err := c.Do(ctx, http.MethodGet, path, q, nil, &page); err != nil {
			return err
		}
		if visit(page.Data) || page.NextCursor == nil || *page.NextCursor == "" {
			return nil
		}
		if seen[*page.NextCursor] { // defensive: a server bug must not loop forever
			return nil
		}
		seen[*page.NextCursor] = true
		q.Set("cursor", *page.NextCursor)
	}
}

// ListAll merges every page into one slice.
func ListAll[T any](ctx context.Context, c *Client, path string, query url.Values) ([]T, error) {
	var all []T
	err := EachPage(ctx, c, path, query, func(items []T) bool {
		all = append(all, items...)
		return false
	})
	return all, err
}

// Find returns the first item that matches, scanning pages until found (nil when absent).
func Find[T any](ctx context.Context, c *Client, path string, query url.Values, match func(T) bool) (*T, error) {
	var found *T
	err := EachPage(ctx, c, path, query, func(items []T) bool {
		for i := range items {
			if match(items[i]) {
				found = &items[i]
				return true
			}
		}
		return false
	})
	return found, err
}
