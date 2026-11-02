// Package authn issues and verifies the platform's access tokens.
//
// Design: the auth service is the only holder of the Ed25519 signing key. It
// signs short-lived JWTs (alg EdDSA) and publishes the public keys as a JWKS
// document at /.well-known/jwks.json. Every other service verifies tokens
// locally with a Verifier that fetches and caches that JWKS, so services stay
// stateless and never call auth on the hot path.
//
// Claims carry the caller's role plus the tenant it belongs to (fleet org or
// station) and its "actor" id (driver id or staff id). Handlers authorize on
// these claims; they never trust IDs in request bodies for ownership checks.
package authn

import (
	"context"
	"slices"

	"github.com/golang-jwt/jwt/v5"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/i18n"
)

// Role is a platform role. One user has exactly one role.
type Role string

const (
	RoleDriver       Role = "driver"        // fleet driver using the mobile app
	RoleFleetManager Role = "fleet_manager" // fleet company admin (mobile + web)
	RoleAttendant    Role = "attendant"     // station pump attendant (mobile scanner)
	RoleStationAdmin Role = "station_admin" // station supervisor (web portal)
)

// Valid reports whether r is a known role.
func (r Role) Valid() bool {
	return slices.Contains([]Role{RoleDriver, RoleFleetManager, RoleAttendant, RoleStationAdmin}, r)
}

// IsStationStaff reports whether r works at a station (attendant or admin).
func (r Role) IsStationStaff() bool { return r == RoleAttendant || r == RoleStationAdmin }

// Default token audience and issuer.
const (
	TokenIssuer   = "petro-auth"
	TokenAudience = "petro-api"
)

// Claims are the JWT claims of an access token.
type Claims struct {
	jwt.RegisteredClaims
	Role    Role      `json:"role"`
	Org     string    `json:"org,omitempty"` // fleet org id (drivers, fleet managers)
	Station string    `json:"stn,omitempty"` // station id (attendants, station admins)
	Actor   string    `json:"act,omitempty"` // driver id or staff id
	Name    i18n.Text `json:"name"`          // display name, for logs/UI only
}

type ctxKey struct{}

// WithClaims returns ctx carrying c.
func WithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, ctxKey{}, c)
}

// FromContext returns the verified claims of the current request, or nil.
func FromContext(ctx context.Context) *Claims {
	c, _ := ctx.Value(ctxKey{}).(*Claims)
	return c
}

type tokenKey struct{}

// WithToken stores the raw bearer token so it can be forwarded to other
// services on behalf of the caller (see svcclient).
func WithToken(ctx context.Context, tok string) context.Context {
	return context.WithValue(ctx, tokenKey{}, tok)
}

// TokenFrom returns the raw bearer token stored by the middleware, or "".
func TokenFrom(ctx context.Context) string {
	t, _ := ctx.Value(tokenKey{}).(string)
	return t
}
