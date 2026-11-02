// Package db provides Postgres access for the services: a pgx connection pool,
// embedded SQL migrations run on startup, transaction helpers, and mapping of
// Postgres constraint violations to API errors.
//
// Each service owns its own database (database-per-service). Migrations are
// plain SQL files managed by goose and embedded into the service binary, so a
// deployed image always carries exactly the schema it expects. A Postgres
// advisory lock makes concurrent replicas run migrations one at a time.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// Open connects to Postgres, retrying until ctx is done. Retrying lets a
// service start before its database is ready (compose/k8s start order).
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("db: parse dsn: %w", err)
	}
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	for attempt := 1; ; attempt++ {
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				return pool, nil
			}
			pool.Close()
		}
		slog.Warn("waiting for database", "attempt", attempt, "err", err)
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("db: connect: %w", err)
		case <-time.After(min(time.Duration(attempt)*time.Second, 5*time.Second)):
		}
	}
}

// Migrate applies the goose migrations found in fsys (root directory of the
// FS) under a session-level advisory lock.
func Migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, fsys, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("db: migrations: %w", err)
	}
	res, err := p.Up(ctx)
	if err != nil {
		return fmt.Errorf("db: migrate up: %w", err)
	}
	for _, r := range res {
		slog.Info("migration applied", "version", r.Source.Version, "file", r.Source.Path, "ms", r.Duration.Milliseconds())
	}
	return nil
}

// Exec runs a multi-statement SQL script inside a transaction. Used for the
// idempotent demo seed scripts.
func Exec(ctx context.Context, pool *pgxpool.Pool, script string) error {
	return InTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, script)
		return err
	})
}

// InTx runs fn in a transaction, committing on success and rolling back on
// error or panic.
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) (err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Postgres SQLSTATE codes we translate into domain errors.
const (
	codeUniqueViolation     = "23505"
	codeForeignKeyViolation = "23503"
)

// IsUniqueViolation reports whether err is a unique-constraint violation,
// optionally restricted to the named constraint.
func IsUniqueViolation(err error, constraint string) bool {
	return isPgCode(err, codeUniqueViolation, constraint)
}

// IsForeignKeyViolation reports whether err is a foreign-key violation.
func IsForeignKeyViolation(err error) bool { return isPgCode(err, codeForeignKeyViolation, "") }

func isPgCode(err error, code, constraint string) bool {
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != code {
		return false
	}
	return constraint == "" || pe.ConstraintName == constraint
}

// IsNoRows reports whether err means "no rows in result set".
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) }

// Ping is a readiness check for the pool.
func Ping(pool *pgxpool.Pool) func(context.Context) error {
	return func(ctx context.Context) error { return pool.Ping(ctx) }
}
