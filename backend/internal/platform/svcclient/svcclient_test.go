package svcclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
)

func TestForwardsTokenAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("authorization header = %q", r.Header.Get("Authorization"))
		}
		var in map[string]int
		_ = json.NewDecoder(r.Body).Decode(&in)
		_ = httpx.OK(w, map[string]int{"got": in["n"]})
	}))
	defer srv.Close()

	ctx := authn.WithToken(context.Background(), "tok")
	var out map[string]int
	if err := New(srv.URL).Post(ctx, "/x", map[string]int{"n": 7}, &out); err != nil {
		t.Fatal(err)
	}
	if out["got"] != 7 {
		t.Fatalf("out = %v", out)
	}
}

func TestPassesThroughClientErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, r, httpx.Conflict("limit_exceeded", "تجاوز", "Exceeded"))
	}))
	defer srv.Close()

	err := New(srv.URL).Get(context.Background(), "/x", nil)
	e := httpx.AsError(err)
	if e == nil || e.Status != http.StatusConflict || e.Code != "limit_exceeded" || e.Message.Ar != "تجاوز" {
		t.Fatalf("err = %#v", err)
	}
	if StatusOf(err) != http.StatusConflict {
		t.Fatalf("StatusOf = %d", StatusOf(err))
	}
}

func TestServerErrorsBecomeUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, r, httpx.ErrInternal)
	}))
	defer srv.Close()
	if s := StatusOf(New(srv.URL).Get(context.Background(), "/x", nil)); s != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", s)
	}
}

func TestUnreachableUpstream(t *testing.T) {
	if s := StatusOf(New("http://127.0.0.1:1").Get(context.Background(), "/x", nil)); s != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", s)
	}
}
