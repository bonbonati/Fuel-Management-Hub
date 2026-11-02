package auth

import (
	"net"
	"net/http"
	"strings"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
)

// API exposes the auth service over HTTP.
type API struct {
	svc    *Service
	issuer *authn.Issuer
}

// NewAPI returns the HTTP adapter.
func NewAPI(svc *Service, issuer *authn.Issuer) *API { return &API{svc: svc, issuer: issuer} }

// Register adds the routes to mux.
func (a *API) Register(mux *http.ServeMux) {
	const p = "/api/v1/auth"
	mux.HandleFunc("POST "+p+"/login", httpx.Handle(a.login))
	mux.HandleFunc("POST "+p+"/demo-login", httpx.Handle(a.demoLogin))
	mux.HandleFunc("GET "+p+"/demo-accounts", httpx.Handle(a.demoAccounts))
	mux.HandleFunc("GET "+p+"/me", authn.Require(a.me))
	// Public keys for token verification by the other services.
	mux.HandleFunc("GET /.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=300")
		_ = httpx.OK(w, a.issuer.JWKS())
	})
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *API) login(w http.ResponseWriter, r *http.Request) error {
	var in credentials
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	res, err := a.svc.Login(r.Context(), in.Username, in.Password, clientIP(r))
	if err != nil {
		return err
	}
	return httpx.OK(w, res)
}

func (a *API) demoLogin(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Username string `json:"username"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	res, err := a.svc.DemoLogin(r.Context(), in.Username)
	if err != nil {
		return err
	}
	return httpx.OK(w, res)
}

func (a *API) demoAccounts(w http.ResponseWriter, r *http.Request) error {
	us, err := a.svc.DemoAccounts(r.Context())
	if err != nil {
		return err
	}
	return httpx.OK(w, us)
}

func (a *API) me(w http.ResponseWriter, r *http.Request) error {
	u, err := a.svc.Me(r.Context(), authn.Caller(r).Subject)
	if err != nil {
		return err
	}
	return httpx.OK(w, u)
}

// clientIP returns the client address. Behind the gateway the first
// X-Forwarded-For hop is the client (Traefik appends; it is trusted only
// because services are not reachable except through the gateway).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
