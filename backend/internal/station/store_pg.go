package station

import (
	"context"
	"embed"
	"errors"
	"io/fs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bonbonati/fuel-management-hub/backend/internal/common"
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

// PG is the Postgres implementation of Repo.
type PG struct{ pool *pgxpool.Pool }

// NewPG returns a Repo backed by pool.
func NewPG(pool *pgxpool.Pool) *PG { return &PG{pool: pool} }

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

const stationCols = `s.id, s.name, s.area, s.distance_km::float8, s.wait, s.phone, s.bank_mask,
	(SELECT count(*) FROM pumps p WHERE p.station_id = s.id)::int`

func scanStation(row pgx.Row) (Station, error) {
	var s Station
	err := row.Scan(&s.ID, &s.Name, &s.Area, &s.DistanceKm, &s.Wait, &s.Phone, &s.BankMask, &s.PumpCount)
	return s, notFound(err)
}

func (p *PG) ListStations(ctx context.Context) ([]Station, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+stationCols+` FROM stations s ORDER BY s.seq`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Station, error) { return scanStation(r) })
}

func (p *PG) GetStation(ctx context.Context, id string) (Station, error) {
	return scanStation(p.pool.QueryRow(ctx, `SELECT `+stationCols+` FROM stations s WHERE s.id = $1`, id))
}

func (p *PG) UpdateStation(ctx context.Context, s Station) error {
	_, err := p.pool.Exec(ctx, `UPDATE stations SET name = $2, area = $3, phone = $4 WHERE id = $1`, s.ID, s.Name, s.Area, s.Phone)
	return err
}

func scanPump(row pgx.Row) (Pump, error) {
	var x Pump
	var grades []string
	err := row.Scan(&x.StationID, &x.Number, &grades, &x.FlowRate, &x.Online)
	for _, g := range grades {
		x.Grades = append(x.Grades, common.Fuel(g))
	}
	return x, notFound(err)
}

func (p *PG) ListPumps(ctx context.Context, stationID string) ([]Pump, error) {
	rows, err := p.pool.Query(ctx, `SELECT station_id, number, grades, flow_rate, online FROM pumps WHERE station_id = $1 ORDER BY number`, stationID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Pump, error) { return scanPump(r) })
}

func (p *PG) GetPump(ctx context.Context, stationID string, n int) (Pump, error) {
	return scanPump(p.pool.QueryRow(ctx, `SELECT station_id, number, grades, flow_rate, online FROM pumps WHERE station_id = $1 AND number = $2`, stationID, n))
}

func (p *PG) UpdatePump(ctx context.Context, x Pump) error {
	grades := make([]string, len(x.Grades))
	for i, g := range x.Grades {
		grades[i] = string(g)
	}
	_, err := p.pool.Exec(ctx, `UPDATE pumps SET grades = $3, flow_rate = $4, online = $5 WHERE station_id = $1 AND number = $2`,
		x.StationID, x.Number, grades, x.FlowRate, x.Online)
	return err
}

const staffCols = `id, station_id, name, code, role, shift, device, active`

func scanStaff(row pgx.Row) (Staff, error) {
	var s Staff
	err := row.Scan(&s.ID, &s.StationID, &s.Name, &s.Code, &s.Role, &s.Shift, &s.Device, &s.Active)
	return s, notFound(err)
}

func (p *PG) ListStaff(ctx context.Context, stationID string) ([]Staff, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+staffCols+` FROM staff WHERE station_id = $1 ORDER BY seq`, stationID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Staff, error) { return scanStaff(r) })
}

func (p *PG) GetStaff(ctx context.Context, id string) (Staff, error) {
	return scanStaff(p.pool.QueryRow(ctx, `SELECT `+staffCols+` FROM staff WHERE id = $1`, id))
}

func (p *PG) InsertStaff(ctx context.Context, s Staff) (string, error) {
	var id string
	err := p.pool.QueryRow(ctx, `INSERT INTO staff (station_id, name, code, role, shift, device, active) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		s.StationID, s.Name, s.Code, s.Role, s.Shift, s.Device, s.Active).Scan(&id)
	if db.IsUniqueViolation(err, "staff_station_code_key") {
		return "", errDupCode
	}
	return id, err
}

func (p *PG) UpdateStaff(ctx context.Context, s Staff) error {
	_, err := p.pool.Exec(ctx, `UPDATE staff SET name = $2, code = $3, role = $4, shift = $5, device = $6, active = $7 WHERE id = $1`,
		s.ID, s.Name, s.Code, s.Role, s.Shift, s.Device, s.Active)
	if db.IsUniqueViolation(err, "staff_station_code_key") {
		return errDupCode
	}
	return err
}

func (p *PG) Prices(ctx context.Context) (map[common.Fuel]int64, error) {
	rows, err := p.pool.Query(ctx, `SELECT grade, price FROM fuel_prices`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[common.Fuel]int64{}
	for rows.Next() {
		var g string
		var price int64
		if err := rows.Scan(&g, &price); err != nil {
			return nil, err
		}
		out[common.Fuel(g)] = price
	}
	return out, rows.Err()
}
