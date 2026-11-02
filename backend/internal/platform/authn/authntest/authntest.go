// Package authntest provides helpers for testing HTTP handlers that require
// authentication: a throwaway signing key, a matching verifier, and a test
// server running a service's routes behind the real auth middleware.
package authntest

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn"
)

// Env is a test authentication environment.
type Env struct {
	t        *testing.T
	Issuer   *authn.Issuer
	Verifier *authn.Verifier
}

// New creates an Env with a fresh Ed25519 key.
func New(t *testing.T) *Env {
	t.Helper()
	priv, err := authn.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.Public().(ed25519.PublicKey)
	return &Env{t: t, Issuer: authn.NewIssuer(priv, time.Hour), Verifier: authn.NewVerifier(authn.StaticKeys{authn.KeyID(pub): pub})}
}

// With wraps an existing issuer/verifier pair (e.g. the auth service's own).
func With(t *testing.T, iss *authn.Issuer, v *authn.Verifier) *Env {
	return &Env{t: t, Issuer: iss, Verifier: v}
}

// Token returns a signed token for the given identity.
func (e *Env) Token(role authn.Role, org, station, actor string) string {
	e.t.Helper()
	tok, _, err := e.Issuer.Issue("user-"+actor, authn.Claims{Role: role, Org: org, Station: station, Actor: actor})
	if err != nil {
		e.t.Fatal(err)
	}
	return tok
}

// Server starts an httptest server with register's routes behind the
// authentication middleware. It is closed when the test ends.
func (e *Env) Server(register func(*http.ServeMux)) *Client {
	mux := http.NewServeMux()
	register(mux)
	srv := httptest.NewServer(e.Verifier.Authenticate(mux))
	e.t.Cleanup(srv.Close)
	return &Client{t: e.t, URL: srv.URL}
}

// Client sends JSON requests to a test server.
type Client struct {
	t   *testing.T
	URL string
}

// Do sends method path with body (JSON, may be nil) using token (may be "")
// and decodes a JSON response into out (may be nil). It returns the status.
func (c *Client) Do(method, path, token string, body, out any) int {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.URL+path, rd)
	if err != nil {
		c.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("decode %s %s (%d): %v: %s", method, path, resp.StatusCode, err, raw)
		}
	}
	return resp.StatusCode
}

// Problem is the decoded error body of a failed request.
type Problem struct {
	Status  int    `json:"status"`
	Code    string `json:"code"`
	Message struct {
		Ar string `json:"ar"`
		En string `json:"en"`
	} `json:"message"`
}
