package authn

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
)

// JWK is the JSON Web Key form of an Ed25519 public key (RFC 8037, kty OKP).
type JWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
}

// JWKS is a JSON Web Key Set document.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// KeyID derives a stable key id from a public key (first 8 bytes of SHA-256),
// so rotating the key automatically yields a new kid.
func KeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return base64.RawURLEncoding.EncodeToString(sum[:8])
}

// PublicJWK converts an Ed25519 public key into its JWK representation.
func PublicJWK(pub ed25519.PublicKey) JWK {
	return JWK{Kty: "OKP", Crv: "Ed25519", X: base64.RawURLEncoding.EncodeToString(pub), Kid: KeyID(pub), Alg: "EdDSA", Use: "sig"}
}

// ParseJWKS decodes a JWKS document into a kid → public key map, skipping keys
// that are not Ed25519 signing keys.
func ParseJWKS(data []byte) (map[string]ed25519.PublicKey, error) {
	var set JWKS
	if err := json.Unmarshal(data, &set); err != nil {
		return nil, fmt.Errorf("authn: decode jwks: %w", err)
	}
	out := map[string]ed25519.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "OKP" || k.Crv != "Ed25519" || k.Kid == "" {
			continue
		}
		raw, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			continue
		}
		out[k.Kid] = ed25519.PublicKey(raw)
	}
	if len(out) == 0 {
		return nil, errors.New("authn: jwks contains no usable Ed25519 keys")
	}
	return out, nil
}

// ParsePrivateKeyPEM parses a PKCS#8 PEM-encoded Ed25519 private key, as
// produced by `openssl genpkey -algorithm ed25519`.
func ParsePrivateKeyPEM(data []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("authn: signing key is not PEM encoded")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("authn: parse signing key: %w", err)
	}
	priv, ok := k.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("authn: signing key is not an Ed25519 key")
	}
	return priv, nil
}

// GenerateKey creates a fresh Ed25519 key pair. Used for ephemeral development
// keys and in tests; production loads a persistent key from a secret.
func GenerateKey() (ed25519.PrivateKey, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	return priv, err
}

// EncodePrivateKeyPEM is the inverse of ParsePrivateKeyPEM.
func EncodePrivateKeyPEM(priv ed25519.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}
