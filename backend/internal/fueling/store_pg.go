package fueling

import (
	"context"
	"embed"
	"errors"
	"io/fs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/db"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/events"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
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

// InTx runs fn in a transaction (GetTx locks the row).
func (p *PG) InTx(ctx context.Context, fn func(Repo) error) error {
	return db.InTx(ctx, p.pool, func(tx pgx.Tx) error { return fn(pgRepo{q: tx, lock: true}) })
}

type pgRepo struct {
	q    querier
	lock bool
}

var errCodeClash = httpx.ErrUnavailable.WithDetail("start code clash, retry")

func (r pgRepo) NextTxID(ctx context.Context) (string, error) {
	var id string
	err := r.q.QueryRow(ctx, `SELECT 'T-' || nextval('tx_seq')`).Scan(&id)
	return id, err
}

const txCols = `id, org_id, vehicle_id, driver_id, plate, station_id, pump, fuel, amount, price_per_litre,
	status, code, source, odometer_km, created_at, started_at, completed_at`

func scanTx(row pgx.Row) (Transaction, error) {
	var t Transaction
	err := row.Scan(&t.ID, &t.OrgID, &t.VehicleID, &t.DriverID, &t.Plate, &t.StationID, &t.Pump, &t.Fuel, &t.Amount, &t.PricePerLitre,
		&t.Status, &t.Code, &t.Source, &t.OdometerKm, &t.CreatedAt, &t.StartedAt, &t.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

func (r pgRepo) InsertTx(ctx context.Context, t Transaction) error {
	_, err := r.q.Exec(ctx, `INSERT INTO transactions (`+txCols+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
		t.ID, t.OrgID, t.VehicleID, t.DriverID, t.Plate, t.StationID, t.Pump, t.Fuel, t.Amount, t.PricePerLitre,
		t.Status, t.Code, t.Source, t.OdometerKm, t.CreatedAt, t.StartedAt, t.CompletedAt)
	if db.IsUniqueViolation(err, "transactions_open_code_key") {
		return errCodeClash
	}
	return err
}

func (r pgRepo) GetTx(ctx context.Context, id string) (Transaction, error) {
	q := `SELECT ` + txCols + ` FROM transactions WHERE id = $1`
	if r.lock {
		q += ` FOR UPDATE`
	}
	return scanTx(r.q.QueryRow(ctx, q, id))
}

func (r pgRepo) UpdateTx(ctx context.Context, t Transaction) error {
	_, err := r.q.Exec(ctx, `UPDATE transactions SET status = $2, started_at = $3, completed_at = $4 WHERE id = $1`,
		t.ID, t.Status, t.StartedAt, t.CompletedAt)
	return err
}

func (r pgRepo) ListTx(ctx context.Context, f TxFilter, limit int) ([]Transaction, error) {
	col, val := "station_id", f.StationID
	switch {
	case f.DriverID != "":
		col, val = "driver_id", f.DriverID
	case f.OrgID != "":
		col, val = "org_id", f.OrgID
	}
	// col is chosen from a fixed set above, never from user input.
	rows, err := r.q.Query(ctx, `SELECT `+txCols+` FROM transactions WHERE `+col+` = $1 ORDER BY created_at DESC LIMIT $2`, val, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Transaction, error) { return scanTx(row) })
}

func (r pgRepo) FindOpenByCode(ctx context.Context, stationID, code string) (Transaction, error) {
	q := `SELECT ` + txCols + ` FROM transactions WHERE station_id = $1 AND code = $2 AND code <> '' AND status = 'auth'`
	if r.lock {
		q += ` FOR UPDATE`
	}
	return scanTx(r.q.QueryRow(ctx, q, stationID, code))
}

func (r pgRepo) CodeInUse(ctx context.Context, stationID, code string) (bool, error) {
	var ok bool
	err := r.q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM transactions WHERE station_id = $1 AND code = $2 AND status = 'auth')`, stationID, code).Scan(&ok)
	return ok, err
}

func (r pgRepo) PumpBusy(ctx context.Context, stationID string, pump int) (bool, error) {
	var ok bool
	err := r.q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM transactions WHERE station_id = $1 AND pump = $2 AND status <> 'done')`, stationID, pump).Scan(&ok)
	return ok, err
}

func (r pgRepo) PendingOdometer(ctx context.Context, driverID string) (*OdometerReading, error) {
	var o OdometerReading
	err := r.q.QueryRow(ctx, `SELECT driver_id, km, recorded_at FROM odometer_readings
		WHERE driver_id = $1 AND consumed_by IS NULL ORDER BY recorded_at DESC LIMIT 1`, driverID).Scan(&o.DriverID, &o.Km, &o.RecordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r pgRepo) InsertOdometer(ctx context.Context, o OdometerReading) error {
	_, err := r.q.Exec(ctx, `INSERT INTO odometer_readings (driver_id, km, recorded_at) VALUES ($1, $2, $3)`, o.DriverID, o.Km, o.RecordedAt)
	return err
}

func (r pgRepo) ConsumeOdometer(ctx context.Context, driverID, txID string) error {
	_, err := r.q.Exec(ctx, `UPDATE odometer_readings SET consumed_by = $2 WHERE driver_id = $1 AND consumed_by IS NULL`, driverID, txID)
	return err
}

func (r pgRepo) InsertSecurityEvent(ctx context.Context, e SecurityEvent) error {
	_, err := r.q.Exec(ctx, `INSERT INTO security_events (org_id, severity, title, detail, ref, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		e.OrgID, e.Severity, e.Title, e.Detail, e.Ref, e.CreatedAt)
	return err
}

func (r pgRepo) ListSecurityEvents(ctx context.Context, orgID string, limit int) ([]SecurityEvent, error) {
	rows, err := r.q.Query(ctx, `SELECT id, org_id, severity, title, detail, ref, created_at FROM security_events
		WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (SecurityEvent, error) {
		var e SecurityEvent
		return e, row.Scan(&e.ID, &e.OrgID, &e.Severity, &e.Title, &e.Detail, &e.Ref, &e.CreatedAt)
	})
}

func (r pgRepo) Enqueue(ctx context.Context, ev events.Event) error {
	return events.Enqueue(ctx, r.q, ev)
}
