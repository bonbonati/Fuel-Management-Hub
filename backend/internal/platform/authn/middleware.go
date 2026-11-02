package authn

import (
	"net/http"
	"slices"
	"strings"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
)

// Authenticate verifies the bearer token (when present) and stores the claims
// and the raw token in the request context. Requests without a valid token
// continue anonymously; use Require to protect a route.
func (v *Verifier) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, ok := bearer(r)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		c, err := v.Verify(r.Context(), tok)
		if err != nil {
			httpx.WriteError(w, r, httpx.ErrUnauthorized.WithDetail("invalid or expired token"))
			return
		}
		ctx := WithToken(WithClaims(r.Context(), c), tok)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearer(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if len(h) < 8 || !strings.EqualFold(h[:7], "Bearer ") {
		return "", false
	}
	t := strings.TrimSpace(h[7:])
	return t, t != ""
}

// Require wraps h so that it only runs for authenticated callers holding one
// of roles. Missing/invalid tokens yield 401, a wrong role 403.
func Require(h httpx.HandlerFunc, roles ...Role) http.HandlerFunc {
	return httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		c := FromContext(r.Context())
		if c == nil {
			return httpx.ErrUnauthorized
		}
		if len(roles) > 0 && !slices.Contains(roles, c.Role) {
			return httpx.ErrForbidden
		}
		return h(w, r)
	})
}

// Caller returns the verified claims of the request. It must only be used
// inside handlers wrapped by Require, which guarantees non-nil claims.
func Caller(r *http.Request) *Claims { return FromContext(r.Context()) }
