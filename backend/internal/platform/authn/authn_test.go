package authn

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/i18n"
)

func newIssuer(t *testing.T) (*Issuer, ed25519.PrivateKey) {
	t.Helper()
	priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return NewIssuer(priv, time.Hour), priv
}

func staticVerifier(priv ed25519.PrivateKey) *Verifier {
	pub := priv.Public().(ed25519.PublicKey)
	return NewVerifier(StaticKeys{KeyID(pub): pub})
}

func TestIssueAndVerifyRoundTrip(t *testing.T) {
	iss, priv := newIssuer(t)
	tok, exp, err := iss.Issue("u1", Claims{Role: RoleDriver, Org: "o1", Actor: "d3", Name: i18n.T("عبدالرحمن", "Abdurahman")})
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(exp) <= 0 {
		t.Fatal("expiry must be in the future")
	}
	c, err := staticVerifier(priv).Verify(context.Background(), tok)
	if err != nil {
		t.Fatal(err)
	}
	if c.Subject != "u1" || c.Role != RoleDriver || c.Org != "o1" || c.Actor != "d3" || c.Name.En != "Abdurahman" {
		t.Fatalf("claims = %+v", c)
	}
}

func TestVerifyRejects(t *testing.T) {
	iss, priv := newIssuer(t)
	v := staticVerifier(priv)

	otherIss, _ := newIssuer(t)
	foreign, _, _ := otherIss.Issue("u1", Claims{Role: RoleDriver})

	expiredIss := NewIssuer(priv, time.Hour)
	expiredIss.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	expired, _, _ := expiredIss.Issue("u1", Claims{Role: RoleDriver})

	badRole, _, _ := iss.Issue("u1", Claims{Role: "admin"})

	// An HS256 token must be rejected even if it carries the right kid
	// (guards against algorithm-confusion attacks).
	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{Role: RoleDriver})
	hs.Header["kid"] = KeyID(priv.Public().(ed25519.PublicKey))
	hsTok, _ := hs.SignedString([]byte("secret"))

	cases := map[string]string{
		"garbage":           "not-a-token",
		"signed by another": foreign,
		"expired":           expired,
		"unknown role":      badRole,
		"wrong algorithm":   hsTok,
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := v.Verify(context.Background(), tok); err == nil {
				t.Fatal("expected verification failure")
			}
		})
	}
}

func TestJWKSRoundTrip(t *testing.T) {
	iss, priv := newIssuer(t)
	b, _ := json.Marshal(iss.JWKS())
	keys, err := ParseJWKS(b)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.Public().(ed25519.PublicKey)
	if got := keys[KeyID(pub)]; !got.Equal(pub) {
		t.Fatal("JWKS did not round-trip the public key")
	}
	if _, err := ParseJWKS([]byte(`{"keys":[{"kty":"RSA"}]}`)); err == nil {
		t.Fatal("JWKS without Ed25519 keys must be rejected")
	}
}

func TestPEMRoundTrip(t *testing.T) {
	priv, _ := GenerateKey()
	pemBytes, err := EncodePrivateKeyPEM(priv)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParsePrivateKeyPEM(pemBytes)
	if err != nil || !back.Equal(priv) {
		t.Fatalf("PEM round trip failed: %v", err)
	}
	if _, err := ParsePrivateKeyPEM([]byte("nope")); err == nil {
		t.Fatal("non-PEM input must fail")
	}
}

func TestRemoteJWKSFetchesAndCaches(t *testing.T) {
	iss, priv := newIssuer(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(iss.JWKS())
	}))
	defer srv.Close()

	src := NewRemoteJWKS(srv.URL)
	kid := KeyID(priv.Public().(ed25519.PublicKey))
	for range 3 {
		if _, err := src.Key(context.Background(), kid); err != nil {
			t.Fatal(err)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("expected one fetch, got %d", hits.Load())
	}
	// Unknown kids are throttled: no refetch inside minRefresh.
	if _, err := src.Key(context.Background(), "unknown"); err == nil {
		t.Fatal("unknown kid must fail")
	}
	if hits.Load() != 1 {
		t.Fatalf("unknown kid refetched inside throttle window (%d fetches)", hits.Load())
	}
}

func TestMiddlewareAndRequire(t *testing.T) {
	iss, priv := newIssuer(t)
	v := staticVerifier(priv)
	h := v.Authenticate(Require(func(w http.ResponseWriter, r *http.Request) error {
		if TokenFrom(r.Context()) == "" {
			t.Error("raw token not stored in context")
		}
		return httpx.OK(w, map[string]string{"actor": Caller(r).Actor})
	}, RoleFleetManager))

	call := func(auth string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	fm, _, _ := iss.Issue("u", Claims{Role: RoleFleetManager, Actor: "m1"})
	drv, _, _ := iss.Issue("u", Claims{Role: RoleDriver})

	if rec := call(""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", rec.Code)
	}
	if rec := call("Bearer junk"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token = %d", rec.Code)
	}
	if rec := call("Bearer " + drv); rec.Code != http.StatusForbidden {
		t.Fatalf("wrong role = %d", rec.Code)
	}
	rec := call("bearer " + fm) // scheme is case-insensitive
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "m1") {
		t.Fatalf("authorized = %d %s", rec.Code, rec.Body)
	}
}

func TestRoleHelpers(t *testing.T) {
	if !RoleAttendant.IsStationStaff() || !RoleStationAdmin.IsStationStaff() || RoleDriver.IsStationStaff() {
		t.Fatal("IsStationStaff misclassifies roles")
	}
	if Role("root").Valid() {
		t.Fatal("unknown role reported valid")
	}
}
