package payment

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/db"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrations is the embedded goose migration set.
var Migrations, _ = fs.Sub(migrationFiles, "migrations")

// Seed is the idempotent demo data script.
//
//go:embed seed.sql
var Seed string

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// PG is the Postgres implementation of Store.
type PG struct {
	pgRepo
	pool *pgxpool.Pool
}

// NewPG returns a Store backed by pool.
func NewPG(pool *pgxpool.Pool) *PG { return &PG{pgRepo: pgRepo{q: pool}, pool: pool} }

// InTx runs fn in a transaction in which LockWallet takes a row lock.
func (p *PG) InTx(ctx context.Context, fn func(Repo) error) error {
	return db.InTx(ctx, p.pool, func(tx pgx.Tx) error { return fn(pgRepo{q: tx, lock: true}) })
}

type pgRepo struct {
	q    querier
	lock bool
}

func (r pgRepo) LockWallet(ctx context.Context, orgID string) (int64, error) {
	q := `SELECT balance FROM wallets WHERE org_id = $1`
	if r.lock {
		q += ` FOR UPDATE`
	}
	var b int64
	err := r.q.QueryRow(ctx, q, orgID).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return b, err
}

func (r pgRepo) SetBalance(ctx context.Context, orgID string, balance int64) error {
	_, err := r.q.Exec(ctx, `UPDATE wallets SET balance = $2, updated_at = now() WHERE org_id = $1`, orgID, balance)
	return err
}

func (r pgRepo) FindEntry(ctx context.Context, kind, txID string) (Entry, error) {
	var e Entry
	err := r.q.QueryRow(ctx, `SELECT org_id, vehicle_id, tx_id, kind, amount, created_at FROM ledger WHERE kind = $1 AND tx_id = $2`, kind, txID).
		Scan(&e.OrgID, &e.VehicleID, &e.TxID, &e.Kind, &e.Amount, &e.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, ErrNotFound
	}
	return e, err
}

func (r pgRepo) InsertEntry(ctx context.Context, e Entry) error {
	_, err := r.q.Exec(ctx, `INSERT INTO ledger (org_id, vehicle_id, tx_id, kind, amount, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		e.OrgID, e.VehicleID, e.TxID, e.Kind, e.Amount, e.CreatedAt)
	return err
}

// usageSQL sums a vehicle's consumption: opening usage and charges, minus refunds.
const usageSQL = `COALESCE(SUM(CASE WHEN kind = 'refund' THEN -amount ELSE amount END), 0)::bigint`

func (r pgRepo) VehicleUsage(ctx context.Context, orgID, vehicleID string, since time.Time) (int64, error) {
	var n int64
	err := r.q.QueryRow(ctx, `SELECT `+usageSQL+` FROM ledger
		WHERE org_id = $1 AND vehicle_id = $2 AND created_at >= $3 AND kind IN ('opening_usage', 'charge', 'refund')`,
		orgID, vehicleID, since).Scan(&n)
	return n, err
}

func (r pgRepo) OrgUsage(ctx context.Context, orgID string, since time.Time) (map[string]int64, error) {
	rows, err := r.q.Query(ctx, `SELECT vehicle_id, `+usageSQL+` FROM ledger
		WHERE org_id = $1 AND vehicle_id IS NOT NULL AND created_at >= $2 AND kind IN ('opening_usage', 'charge', 'refund')
		GROUP BY vehicle_id`, orgID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var v string
		var n int64
		if err := rows.Scan(&v, &n); err != nil {
			return nil, err
		}
		out[v] = n
	}
	return out, rows.Err()
}

func (r pgRepo) GetTax(ctx context.Context) (Tax, error) {
	var t Tax
	err := r.q.QueryRow(ctx, `SELECT enabled, rate::float8 FROM tax_settings WHERE id = 1`).Scan(&t.Enabled, &t.Rate)
	return t, err
}

func (r pgRepo) SetTax(ctx context.Context, t Tax) error {
	_, err := r.q.Exec(ctx, `UPDATE tax_settings SET enabled = $1, rate = $2 WHERE id = 1`, t.Enabled, t.Rate)
	return err
}

func (r pgRepo) ListSettlements(ctx context.Context, stationID string, limit int) ([]Settlement, error) {
	rows, err := r.q.Query(ctx, `SELECT station_id, to_char(payout_date, 'YYYY-MM-DD'), amount, fee, status
		FROM settlements WHERE station_id = $1 ORDER BY payout_date DESC LIMIT $2`, stationID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Settlement, error) {
		var s Settlement
		return s, row.Scan(&s.StationID, &s.Date, &s.Amount, &s.Fee, &s.Status)
	})
}

func (r pgRepo) AccrueSettlement(ctx context.Context, stationID, date string, amount, fee int64) error {
	_, err := r.q.Exec(ctx, `INSERT INTO settlements (station_id, payout_date, amount, fee, status)
		VALUES ($1, $2::date, $3, $4, 'scheduled')
		ON CONFLICT (station_id, payout_date)
		DO UPDATE SET amount = settlements.amount + EXCLUDED.amount, fee = settlements.fee + EXCLUDED.fee`,
		stationID, date, amount, fee)
	return err
}

func (r pgRepo) MarkProcessed(ctx context.Context, eventID string) (bool, error) {
	tag, err := r.q.Exec(ctx, `INSERT INTO processed_events (id) VALUES ($1) ON CONFLICT DO NOTHING`, eventID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
