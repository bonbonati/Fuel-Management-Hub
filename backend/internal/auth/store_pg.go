package auth

import (
	"context"
	"embed"
	"errors"
	"io/fs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrations is the embedded goose migration set.
var Migrations, _ = fs.Sub(migrationFiles, "migrations")

// PG is the Postgres implementation of Repo.
type PG struct{ pool *pgxpool.Pool }

// NewPG returns a Repo backed by pool.
func NewPG(pool *pgxpool.Pool) *PG { return &PG{pool: pool} }

const userCols = `id::text, username, password_hash, role, org_id, station_id, actor_id, name, active, demo`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.OrgID, &u.StationID, &u.ActorID, &u.Name, &u.Active, &u.Demo)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (p *PG) ByUsername(ctx context.Context, username string) (User, error) {
	return scanUser(p.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE lower(username) = lower($1)`, username))
}

func (p *PG) ByID(ctx context.Context, id string) (User, error) {
	return scanUser(p.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id::text = $1`, id))
}

func (p *PG) Insert(ctx context.Context, u User) (string, error) {
	var id string
	err := p.pool.QueryRow(ctx, `INSERT INTO users (username, password_hash, role, org_id, station_id, actor_id, name, active, demo)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id::text`,
		u.Username, u.PasswordHash, u.Role, u.OrgID, u.StationID, u.ActorID, u.Name, u.Active, u.Demo).Scan(&id)
	return id, err
}

func (p *PG) ListDemo(ctx context.Context) ([]User, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+userCols+` FROM users WHERE demo AND active ORDER BY created_at, username`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (User, error) { return scanUser(r) })
}
