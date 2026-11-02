// Package svcclient is the HTTP client services use to call each other's
// /internal/v1 endpoints.
//
// Calls are made on behalf of the end user: the caller's bearer token (stored
// in the context by authn middleware) and request id are forwarded, so the
// callee authorizes with the same identity and logs share one request id.
// Problem documents returned by the callee are decoded back into *httpx.Error
// so user-facing (bilingual) messages propagate unchanged to the client.
// Internal routes are never exposed through the gateway/Ingress.
package svcclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/i18n"
)

// Client calls one upstream service.
type Client struct {
	base string
	http *http.Client
}

// New returns a client for the service at baseURL (e.g. "http://fleet:8080").
func New(baseURL string) *Client {
	return &Client{base: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 5 * time.Second}}
}

// Get performs GET path and decodes the JSON response into dst.
func (c *Client) Get(ctx context.Context, path string, dst any) error {
	return c.Do(ctx, http.MethodGet, path, nil, dst)
}

// Post performs POST path with body as JSON and decodes the response into dst
// (dst may be nil).
func (c *Client) Post(ctx context.Context, path string, body, dst any) error {
	return c.Do(ctx, http.MethodPost, path, body, dst)
}

// Do performs a request. Transport failures and unexpected responses become
// a 503 *httpx.Error; upstream problem documents are passed through.
func (c *Client) Do(ctx context.Context, method, path string, body, dst any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if tok := authn.TokenFrom(ctx); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if id := httpx.RequestIDFrom(ctx); id != "" {
		req.Header.Set(httpx.RequestIDHeader, id)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return httpx.ErrUnavailable.WithDetail("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return httpx.ErrUnavailable.WithDetail("%s %s: read body: %v", method, path, err)
	}
	if resp.StatusCode >= 400 {
		return decodeProblem(resp.StatusCode, raw, method+" "+path)
	}
	if dst == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return httpx.ErrUnavailable.WithDetail("%s %s: decode: %v", method, path, err)
	}
	return nil
}

// decodeProblem maps an upstream error response to *httpx.Error. 4xx problems
// keep their status and message; anything else becomes 503 (the upstream
// failure is not the caller's fault).
func decodeProblem(status int, raw []byte, what string) error {
	var p struct {
		Code    string    `json:"code"`
		Message i18n.Text `json:"message"`
		Detail  string    `json:"detail"`
	}
	if json.Unmarshal(raw, &p) == nil && p.Code != "" && status < 500 {
		return &httpx.Error{Status: status, Code: p.Code, Message: p.Message, Detail: p.Detail}
	}
	return httpx.ErrUnavailable.WithDetail("%s: upstream status %d", what, status)
}

// Error helper for tests and callers that need the upstream status.
func StatusOf(err error) int {
	if e := httpx.AsError(err); e != nil {
		return e.Status
	}
	return 0
}
