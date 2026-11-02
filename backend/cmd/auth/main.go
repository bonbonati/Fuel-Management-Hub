// Command auth runs the identity service: sign-in, token issuing and the
// JWKS endpoint. See internal/auth.
package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/bonbonati/fuel-management-hub/backend/internal/auth"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/app"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/config"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/db"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/kv"
)

func main() {
	app.Main("auth", func(ctx context.Context) error {
		priv, err := signingKey()
		if err != nil {
			return err
		}
		ttl, err := config.Duration("ACCESS_TOKEN_TTL", 8*time.Hour)
		if err != nil {
			return err
		}
		demo, err := config.Bool("DEMO_MODE", false)
		if err != nil {
			return err
		}
		cache, err := kv.NewValkey(config.String("VALKEY_ADDR", "valkey:6379"), config.String("VALKEY_PASSWORD", ""))
		if err != nil {
			return err
		}
		defer cache.Close()
		pool, err := app.Database(ctx, auth.Migrations, "")
		if err != nil {
			return err
		}
		defer pool.Close()

		issuer := authn.NewIssuer(priv, ttl)
		svc := auth.NewService(auth.NewPG(pool), issuer, cache, auth.Options{DemoMode: demo})
		if seed, _ := config.Bool("SEED_DEMO_DATA", false); seed {
			n, err := svc.SeedDemo(ctx, config.String("DEMO_PASSWORD", "demo1234"))
			if err != nil {
				return err
			}
			slog.Info("demo accounts ready", "created", n)
		}
		// auth verifies its own tokens (for /me) with its own public key.
		pub := priv.Public().(ed25519.PublicKey)
		verifier := authn.NewVerifier(authn.StaticKeys{authn.KeyID(pub): pub})
		checks := map[string]httpx.Check{"postgres": db.Ping(pool), "valkey": cache.Ping}
		return app.Serve(ctx, verifier, checks, auth.NewAPI(svc, issuer).Register)
	})
}

// signingKey loads the Ed25519 key from AUTH_SIGNING_KEY (PEM) or
// AUTH_SIGNING_KEY_FILE. With AUTH_ALLOW_EPHEMERAL_KEY=true (single-replica
// development only) a throwaway key is generated instead; tokens then become
// invalid whenever auth restarts.
func signingKey() (ed25519.PrivateKey, error) {
	pemData := []byte(config.String("AUTH_SIGNING_KEY", ""))
	if path := config.String("AUTH_SIGNING_KEY_FILE", ""); len(pemData) == 0 && path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		pemData = b
	}
	if len(pemData) > 0 {
		return authn.ParsePrivateKeyPEM(pemData)
	}
	if ok, _ := config.Bool("AUTH_ALLOW_EPHEMERAL_KEY", false); ok {
		slog.Warn("using an ephemeral signing key (development only)")
		return authn.GenerateKey()
	}
	return nil, errors.New("auth: set AUTH_SIGNING_KEY or AUTH_SIGNING_KEY_FILE (or AUTH_ALLOW_EPHEMERAL_KEY=true for development)")
}
