package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestWriteErrorRendersTypedError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	WriteError(rec, req, fmt.Errorf("wrapped: %w", Conflict("dup", "مكرر", "Duplicate")))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("content type = %q", ct)
	}
	var p problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Code != "dup" || p.Message.Ar != "مكرر" || p.Message.En != "Duplicate" || p.Status != 409 {
		t.Fatalf("problem = %+v", p)
	}
}

func TestWriteErrorHidesUnknownErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, httptest.NewRequest(http.MethodGet, "/x", nil), errors.New("db password is hunter2"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "hunter2") {
		t.Fatal("internal error details leaked to the client")
	}
}

func TestDecodeJSON(t *testing.T) {
	type in struct {
		Name string `json:"name"`
	}
	tests := []struct {
		name, body string
		wantErr    bool
	}{
		{"valid", `{"name":"x"}`, false},
		{"unknown field rejected", `{"name":"x","extra":1}`, true},
		{"empty body rejected", ``, true},
		{"malformed rejected", `{"name":`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var v in
			err := DecodeJSON(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body)), &v)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && AsError(err).Code != "invalid_json" {
				t.Fatalf("code = %s", AsError(err).Code)
			}
		})
	}
}

func TestHandleWritesReturnedError(t *testing.T) {
	h := Handle(func(http.ResponseWriter, *http.Request) error { return ErrForbidden })
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRequestIDGeneratedAndPropagated(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = RequestIDFrom(r.Context()) }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if seen == "" || rec.Header().Get(RequestIDHeader) != seen {
		t.Fatalf("generated id %q not echoed (%q)", seen, rec.Header().Get(RequestIDHeader))
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "abc")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "abc" {
		t.Fatalf("inbound id not reused, got %q", seen)
	}
}

func TestRecoverConvertsPanic(t *testing.T) {
	h := Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCORS(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := CORS([]string{"capacitor://localhost"})(next)

	pre := httptest.NewRequest(http.MethodOptions, "/", nil)
	pre.Header.Set("Origin", "capacitor://localhost")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, pre)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "capacitor://localhost" {
		t.Fatalf("preflight: status %d, allow-origin %q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}

	other := httptest.NewRequest(http.MethodGet, "/", nil)
	other.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, other)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Code != http.StatusTeapot {
		t.Fatal("disallowed origin must not receive CORS headers")
	}
}

func TestChainOrder(t *testing.T) {
	var order []string
	mk := func(n string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, n)
				next.ServeHTTP(w, r)
			})
		}
	}
	Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), mk("a"), mk("b")).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Join(order, ",") != "a,b" {
		t.Fatalf("order = %v", order)
	}
}

func TestNewMuxHealthAndReadiness(t *testing.T) {
	reg := prometheus.NewRegistry()
	failing := false
	mux := NewMux(reg, map[string]Check{"db": func(context.Context) error {
		if failing {
			return errors.New("down")
		}
		return nil
	}})
	srv := httptest.NewServer(Metrics(reg)(mux))
	defer srv.Close()

	for path, want := range map[string]int{"/healthz": 200, "/readyz": 200, "/metrics": 200} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("%s = %d", path, resp.StatusCode)
		}
	}
	failing = true
	resp, err := http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readyz with failing check = %d", resp.StatusCode)
	}
}
