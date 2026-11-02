package auth

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn/authntest"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/kv"
)

type memRepo struct{ users []User }

func (m *memRepo) ByUsername(_ context.Context, name string) (User, error) {
	for _, u := range m.users {
		if strings.EqualFold(u.Username, name) {
			return u, nil
		}
	}
	return User{}, ErrNotFound
}
func (m *memRepo) ByID(_ context.Context, id string) (User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return User{}, ErrNotFound
}
func (m *memRepo) Insert(_ context.Context, u User) (string, error) {
	u.ID = fmt.Sprintf("u%d", len(m.users)+1)
	m.users = append(m.users, u)
	return u.ID, nil
}
func (m *memRepo) ListDemo(context.Context) ([]User, error) {
	var out []User
	for _, u := range m.users {
		if u.Demo && u.Active {
			out = append(out, u)
		}
	}
	return out, nil
}

var ctx = context.Background()

type fixture struct {
	svc      *Service
	repo     *memRepo
	issuer   *authn.Issuer
	verifier *authn.Verifier
	limiter  *kv.Memory
}

func newFixture(t *testing.T, demo bool) fixture {
	t.Helper()
	priv, _ := authn.GenerateKey()
	pub := priv.Public().(ed25519.PublicKey)
	iss := authn.NewIssuer(priv, time.Hour)
	repo := &memRepo{}
	lim := kv.NewMemory()
	svc := NewService(repo, iss, lim, Options{DemoMode: demo, BcryptCost: 4}) // low cost keeps tests fast
	if _, err := svc.SeedDemo(ctx, "demo1234"); err != nil {
		t.Fatal(err)
	}
	return fixture{svc, repo, iss, authn.NewVerifier(authn.StaticKeys{authn.KeyID(pub): pub}), lim}
}

func code(err error) string {
	if e := httpx.AsError(err); e != nil {
		return e.Code
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func TestSeedDemoIsIdempotentAndHashes(t *testing.T) {
	f := newFixture(t, true)
	if len(f.repo.users) != len(DemoPersonas) {
		t.Fatalf("seeded %d users", len(f.repo.users))
	}
	for _, u := range f.repo.users {
		if u.PasswordHash == "" || u.PasswordHash == "demo1234" || !u.Demo || !u.Active {
			t.Fatalf("bad seeded user %+v", u)
		}
	}
	n, err := f.svc.SeedDemo(ctx, "other")
	if err != nil || n != 0 {
		t.Fatalf("second seed created %d users (%v)", n, err)
	}
}

func TestLoginIssuesVerifiableTokenWithClaims(t *testing.T) {
	f := newFixture(t, false)
	res, err := f.svc.Login(ctx, "d-3071", "demo1234", "10.0.0.1") // username is case-insensitive
	if err != nil {
		t.Fatal(err)
	}
	c, err := f.verifier.Verify(ctx, res.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if c.Role != authn.RoleDriver || c.Org != "o1" || c.Actor != "d3" || c.Subject != res.User.ID || res.TokenType != "Bearer" {
		t.Fatalf("claims = %+v", c)
	}
	att, _ := f.svc.Login(ctx, "ATT-09", "demo1234", "")
	if c, _ := f.verifier.Verify(ctx, att.AccessToken); c.Station != "s1" || c.Role != authn.RoleAttendant {
		t.Fatalf("attendant claims = %+v", c)
	}
}

func TestLoginFailures(t *testing.T) {
	f := newFixture(t, false)
	f.repo.users[0].Active = false
	inactive := f.repo.users[0].Username
	cases := map[string][2]string{
		"wrong password": {"D-3071", "nope"},
		"unknown user":   {"ghost", "demo1234"},
		"inactive user":  {inactive, "demo1234"},
		"empty username": {"  ", "demo1234"},
		"huge password":  {"D-3071", strings.Repeat("x", 100)},
	}
	for name, cred := range cases {
		if _, err := f.svc.Login(ctx, cred[0], cred[1], ""); code(err) != "invalid_credentials" {
			t.Errorf("%s: code %q", name, code(err))
		}
	}
}

func TestLoginThrottling(t *testing.T) {
	f := newFixture(t, false)
	for range maxPerUsername {
		_, _ = f.svc.Login(ctx, "D-3071", "wrong", "")
	}
	if _, err := f.svc.Login(ctx, "D-3071", "demo1234", ""); code(err) != "too_many_attempts" {
		t.Fatalf("code = %q, want throttled even with the right password", code(err))
	}
	// Window expiry lifts the throttle.
	f.limiter.Now = func() time.Time { return time.Now().Add(throttleWindow + time.Second) }
	if _, err := f.svc.Login(ctx, "D-3071", "demo1234", ""); err != nil {
		t.Fatalf("after window: %v", err)
	}
}

func TestSuccessfulLoginResetsUsernameCounter(t *testing.T) {
	f := newFixture(t, false)
	for range maxPerUsername - 1 {
		_, _ = f.svc.Login(ctx, "D-3071", "wrong", "")
	}
	if _, err := f.svc.Login(ctx, "D-3071", "demo1234", ""); err != nil {
		t.Fatal(err)
	}
	for range maxPerUsername - 1 {
		_, _ = f.svc.Login(ctx, "D-3071", "wrong", "")
	}
	if _, err := f.svc.Login(ctx, "D-3071", "demo1234", ""); err != nil {
		t.Fatalf("counter not reset after success: %v", err)
	}
}

func TestDemoLogin(t *testing.T) {
	off := newFixture(t, false)
	if _, err := off.svc.DemoLogin(ctx, "fleet.manager"); code(err) != "demo_disabled" {
		t.Fatalf("demo disabled: code %q", code(err))
	}
	if us, _ := off.svc.DemoAccounts(ctx); len(us) != 0 {
		t.Fatal("demo accounts must be hidden when demo mode is off")
	}

	on := newFixture(t, true)
	res, err := on.svc.DemoLogin(ctx, "fleet.manager")
	if err != nil || res.User.Role != authn.RoleFleetManager {
		t.Fatalf("demo login = %+v, %v", res.User, err)
	}
	// Non-demo accounts can never be used password-less.
	_, _ = on.repo.Insert(ctx, User{Username: "real", Role: authn.RoleDriver, Active: true})
	if _, err := on.svc.DemoLogin(ctx, "real"); code(err) != "invalid_credentials" {
		t.Fatalf("non-demo user: code %q", code(err))
	}
	if us, _ := on.svc.DemoAccounts(ctx); len(us) != len(DemoPersonas) {
		t.Fatalf("demo accounts = %d", len(us))
	}
}

func TestHTTP(t *testing.T) {
	f := newFixture(t, true)
	srv := authntest.With(t, f.issuer, f.verifier).Server(NewAPI(f.svc, f.issuer).Register)

	var res TokenResponse
	if s := srv.Do("POST", "/api/v1/auth/login", "", credentials{Username: "ATT-01", Password: "demo1234"}, &res); s != 200 || res.AccessToken == "" {
		t.Fatalf("login status %d", s)
	}
	var me User
	if s := srv.Do("GET", "/api/v1/auth/me", res.AccessToken, nil, &me); s != 200 || me.Role != authn.RoleStationAdmin || me.PasswordHash != "" {
		t.Fatalf("me status %d, %+v", s, me)
	}
	if s := srv.Do("GET", "/api/v1/auth/me", "", nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("anonymous me = %d", s)
	}
	var jwks authn.JWKS
	if srv.Do("GET", "/.well-known/jwks.json", "", nil, &jwks); len(jwks.Keys) != 1 || jwks.Keys[0].Alg != "EdDSA" {
		t.Fatalf("jwks = %+v", jwks)
	}
	var p authntest.Problem
	if s := srv.Do("POST", "/api/v1/auth/login", "", credentials{Username: "ATT-01", Password: "x"}, &p); s != 401 || p.Code != "invalid_credentials" {
		t.Fatalf("bad login %d %+v", s, p)
	}
	var demo []User
	if srv.Do("GET", "/api/v1/auth/demo-accounts", "", nil, &demo); len(demo) != len(DemoPersonas) {
		t.Fatalf("demo accounts = %d", len(demo))
	}
}

func TestClientIP(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.1.2.3:5555"
	if clientIP(r) != "10.1.2.3" {
		t.Fatalf("remote addr ip = %q", clientIP(r))
	}
	r.Header.Set("X-Forwarded-For", "41.1.1.1, 10.0.0.2")
	if clientIP(r) != "41.1.1.1" {
		t.Fatalf("forwarded ip = %q", clientIP(r))
	}
}
