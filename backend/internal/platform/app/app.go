// Package app wires the cross-cutting pieces every service's main() needs:
// logging, signal handling, the container healthcheck sub-command, database
// bootstrap (connect → migrate → optional demo seed), token verification and
// the HTTP middleware stack. Keeping this in one place makes every service's
// main.go a short, readable composition root.
package app

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/config"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/db"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
)

// Main runs a service. It handles `<binary> healthcheck` (used by the
// container HEALTHCHECK), configures logging, cancels ctx on SIGINT/SIGTERM
// and exits non-zero when run returns an error.
func Main(name string, run func(ctx context.Context) error) {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(httpx.HealthcheckMain(Port()))
	}
	httpx.SetupLogger(name, config.String("LOG_LEVEL", "info"))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("service stopped with error", "err", err)
		os.Exit(1)
	}
	slog.Info("service stopped")
}

// Port is the HTTP port every service listens on (PORT, default 8080).
func Port() string { return config.String("PORT", "8080") }

// Database connects to DATABASE_URL, applies the embedded migrations and,
// when SEED_DEMO_DATA=true, runs the idempotent demo seed script.
func Database(ctx context.Context, migrations fs.FS, seed string) (*pgxpool.Pool, error) {
	dsn, err := config.Required("DATABASE_URL")
	if err != nil {
		return nil, err
	}
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations); err != nil {
		pool.Close()
		return nil, err
	}
	if doSeed, err := config.Bool("SEED_DEMO_DATA", false); err != nil {
		pool.Close()
		return nil, err
	} else if doSeed && seed != "" {
		if err := db.Exec(ctx, pool, seed); err != nil {
			pool.Close()
			return nil, err
		}
		slog.Info("demo data seeded")
	}
	return pool, nil
}

// Verifier returns a token verifier backed by the auth service's JWKS
// (AUTH_JWKS_URL, default http://auth:8080/.well-known/jwks.json).
func Verifier() *authn.Verifier {
	return authn.NewVerifier(authn.NewRemoteJWKS(config.String("AUTH_JWKS_URL", "http://auth:8080/.well-known/jwks.json")))
}

// Serve builds the standard middleware stack around the service routes and
// serves until ctx is cancelled. register adds the service's routes to mux.
//
// Stack (outermost first): request id → panic recovery → access log →
// metrics → CORS → authentication (verifies bearer tokens when present).
func Serve(ctx context.Context, v *authn.Verifier, checks map[string]httpx.Check, register func(mux *http.ServeMux)) error {
	reg := httpx.NewRegistry()
	mux := httpx.NewMux(reg, checks)
	register(mux)
	var authenticate httpx.Middleware = func(h http.Handler) http.Handler { return h }
	if v != nil {
		authenticate = v.Authenticate
	}
	h := httpx.Chain(mux,
		httpx.RequestID,
		httpx.Recover,
		httpx.Logger,
		httpx.Metrics(prometheus.Registerer(reg)),
		httpx.CORS(config.List("CORS_ALLOWED_ORIGINS", nil)),
		authenticate,
	)
	return httpx.Serve(ctx, ":"+Port(), h)
}
