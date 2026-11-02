package authn

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Issuer signs access tokens. Only the auth service constructs one.
type Issuer struct {
	priv ed25519.PrivateKey
	kid  string
	ttl  time.Duration
	now  func() time.Time
}

// NewIssuer returns an Issuer signing with priv; tokens live for ttl.
func NewIssuer(priv ed25519.PrivateKey, ttl time.Duration) *Issuer {
	pub := priv.Public().(ed25519.PublicKey)
	return &Issuer{priv: priv, kid: KeyID(pub), ttl: ttl, now: time.Now}
}

// Issue signs a token for subject with the given claims template. The
// registered claims (iss, aud, sub, iat, exp, jti) are filled in here.
func (i *Issuer) Issue(subject string, c Claims) (token string, expires time.Time, err error) {
	now := i.now()
	expires = now.Add(i.ttl)
	jti := make([]byte, 12)
	_, _ = rand.Read(jti)
	c.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    TokenIssuer,
		Audience:  jwt.ClaimStrings{TokenAudience},
		Subject:   subject,
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)), // tolerate small clock skew
		ExpiresAt: jwt.NewNumericDate(expires),
		ID:        hex.EncodeToString(jti),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodEdDSA, &c)
	t.Header["kid"] = i.kid
	token, err = t.SignedString(i.priv)
	return token, expires, err
}

// JWKS returns the public key set to publish at /.well-known/jwks.json.
func (i *Issuer) JWKS() JWKS {
	return JWKS{Keys: []JWK{PublicJWK(i.priv.Public().(ed25519.PublicKey))}}
}

// KeySource resolves a key id to a public key.
type KeySource interface {
	Key(ctx context.Context, kid string) (ed25519.PublicKey, error)
}

// StaticKeys is a fixed KeySource (used by the auth service itself and tests).
type StaticKeys map[string]ed25519.PublicKey

// Key implements KeySource.
func (s StaticKeys) Key(_ context.Context, kid string) (ed25519.PublicKey, error) {
	if k, ok := s[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("authn: unknown key id %q", kid)
}

// RemoteJWKS fetches keys from the auth service's JWKS endpoint and caches
// them. Unknown key ids trigger a refetch (handles key rotation), throttled
// to one fetch per minRefresh so a flood of bad tokens cannot DoS auth.
type RemoteJWKS struct {
	url        string
	client     *http.Client
	minRefresh time.Duration

	mu      sync.Mutex
	keys    map[string]ed25519.PublicKey
	fetched time.Time
}

// NewRemoteJWKS returns a KeySource backed by the JWKS document at url.
func NewRemoteJWKS(url string) *RemoteJWKS {
	return &RemoteJWKS{url: url, client: &http.Client{Timeout: 5 * time.Second}, minRefresh: 10 * time.Second}
}

// Key implements KeySource.
func (r *RemoteJWKS) Key(ctx context.Context, kid string) (ed25519.PublicKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if k, ok := r.keys[kid]; ok {
		return k, nil
	}
	if time.Since(r.fetched) < r.minRefresh && r.keys != nil {
		return nil, fmt.Errorf("authn: unknown key id %q", kid)
	}
	keys, err := r.fetch(ctx)
	r.fetched = time.Now()
	if err != nil {
		slog.WarnContext(ctx, "jwks fetch failed", "url", r.url, "err", err)
		return nil, err
	}
	r.keys = keys
	if k, ok := keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("authn: unknown key id %q", kid)
}

func (r *RemoteJWKS) fetch(ctx context.Context) (map[string]ed25519.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("authn: jwks status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	return ParseJWKS(body)
}

// Verifier validates access tokens.
type Verifier struct {
	keys KeySource
}

// NewVerifier returns a Verifier resolving signing keys through keys.
func NewVerifier(keys KeySource) *Verifier { return &Verifier{keys: keys} }

// ErrInvalidToken is returned for any token that fails verification.
var ErrInvalidToken = errors.New("authn: invalid token")

// Verify parses and validates token: signature (EdDSA only), issuer,
// audience, expiry, and that the role is known.
func (v *Verifier) Verify(ctx context.Context, token string) (*Claims, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return v.keys.Key(ctx, kid)
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(TokenIssuer),
		jwt.WithAudience(TokenAudience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if !c.Role.Valid() {
		return nil, fmt.Errorf("%w: unknown role %q", ErrInvalidToken, c.Role)
	}
	return &c, nil
}
