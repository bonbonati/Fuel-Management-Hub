package fleet

import (
	"context"
	"embed"
	_ "embed"
	"errors"
	"io/fs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/db"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrations is the embedded goose migration set (root of the FS).
var Migrations, _ = fs.Sub(migrationFiles, "migrations")

// Seed is the idempotent demo data script.
//
//go:embed seed.sql
var Seed string

// querier is satisfied by both *pgxpool.Pool and pgx.Tx.
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

// InTx runs fn with a transactional Repo whose getters lock rows.
func (p *PG) InTx(ctx context.Context, fn func(Repo) error) error {
	return db.InTx(ctx, p.pool, func(tx pgx.Tx) error { return fn(pgRepo{q: tx, lock: true}) })
}

type pgRepo struct {
	q    querier
	lock bool // append FOR UPDATE to single-row reads (inside transactions)
}

func (r pgRepo) forUpdate() string {
	if r.lock {
		return " FOR UPDATE"
	}
	return ""
}

// one scans a single row, mapping "no rows" to ErrNotFound.
func one[T any](row pgx.Row, scan func(pgx.Row) (T, error)) (T, error) {
	v, err := scan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}

// many scans all rows with scan.
func many[T any](rows pgx.Rows, err error, scan func(pgx.Row) (T, error)) ([]T, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (T, error) { return scan(row) })
}

// ---------- org ----------

func (r pgRepo) GetOrg(ctx context.Context, id string) (Org, error) {
	return one(r.q.QueryRow(ctx, `SELECT id, name, phone, city FROM orgs WHERE id = $1`+r.forUpdate(), id),
		func(row pgx.Row) (Org, error) {
			var o Org
			return o, row.Scan(&o.ID, &o.Name, &o.Phone, &o.City)
		})
}

func (r pgRepo) UpdateOrg(ctx context.Context, o Org) error {
	_, err := r.q.Exec(ctx, `UPDATE orgs SET name = $2, phone = $3, city = $4 WHERE id = $1`, o.ID, o.Name, o.Phone, o.City)
	return err
}

// ---------- tiers ----------

const tierCols = `id, org_id, icon, color, name, budget, daily_litres, fuel_window`

func scanTier(row pgx.Row) (Tier, error) {
	var t Tier
	return t, row.Scan(&t.ID, &t.OrgID, &t.Icon, &t.Color, &t.Name, &t.Budget, &t.DailyLitres, &t.Window)
}

func (r pgRepo) ListTiers(ctx context.Context, orgID string) ([]Tier, error) {
	rows, err := r.q.Query(ctx, `SELECT `+tierCols+` FROM tiers WHERE org_id = $1 ORDER BY seq`, orgID)
	return many(rows, err, scanTier)
}

func (r pgRepo) GetTier(ctx context.Context, id string) (Tier, error) {
	return one(r.q.QueryRow(ctx, `SELECT `+tierCols+` FROM tiers WHERE id = $1`+r.forUpdate(), id), scanTier)
}

func (r pgRepo) InsertTier(ctx context.Context, t Tier) (string, error) {
	var id string
	err := r.q.QueryRow(ctx, `INSERT INTO tiers (org_id, icon, color, name, budget, daily_litres, fuel_window)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		t.OrgID, t.Icon, t.Color, t.Name, t.Budget, t.DailyLitres, t.Window).Scan(&id)
	return id, err
}

func (r pgRepo) UpdateTier(ctx context.Context, t Tier) error {
	_, err := r.q.Exec(ctx, `UPDATE tiers SET icon = $2, color = $3, name = $4, budget = $5, daily_litres = $6, fuel_window = $7 WHERE id = $1`,
		t.ID, t.Icon, t.Color, t.Name, t.Budget, t.DailyLitres, t.Window)
	return err
}

// ---------- drivers ----------

const driverCols = `id, org_id, name, phone, code, app_activated, active`

func scanDriver(row pgx.Row) (Driver, error) {
	var d Driver
	return d, row.Scan(&d.ID, &d.OrgID, &d.Name, &d.Phone, &d.Code, &d.AppActivated, &d.Active)
}

func (r pgRepo) ListDrivers(ctx context.Context, orgID string) ([]Driver, error) {
	rows, err := r.q.Query(ctx, `SELECT `+driverCols+` FROM drivers WHERE org_id = $1 ORDER BY seq`, orgID)
	return many(rows, err, scanDriver)
}

func (r pgRepo) GetDriver(ctx context.Context, id string) (Driver, error) {
	return one(r.q.QueryRow(ctx, `SELECT `+driverCols+` FROM drivers WHERE id = $1`+r.forUpdate(), id), scanDriver)
}

func (r pgRepo) InsertDriver(ctx context.Context, d Driver) (string, error) {
	var id string
	err := r.q.QueryRow(ctx, `INSERT INTO drivers (org_id, name, phone, code, app_activated, active)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		d.OrgID, d.Name, d.Phone, d.Code, d.AppActivated, d.Active).Scan(&id)
	if db.IsUniqueViolation(err, "drivers_org_code_key") {
		return "", errDuplicateCode
	}
	return id, err
}

func (r pgRepo) UpdateDriver(ctx context.Context, d Driver) error {
	_, err := r.q.Exec(ctx, `UPDATE drivers SET name = $2, phone = $3, code = $4, app_activated = $5, active = $6 WHERE id = $1`,
		d.ID, d.Name, d.Phone, d.Code, d.AppActivated, d.Active)
	if db.IsUniqueViolation(err, "drivers_org_code_key") {
		return errDuplicateCode
	}
	return err
}

// ---------- vehicles ----------

const vehicleCols = `id, org_id, plate, name, driver_id, fuel, tier_id, custom_limit, extra, active, tag_battery`

func scanVehicle(row pgx.Row) (Vehicle, error) {
	var v Vehicle
	return v, row.Scan(&v.ID, &v.OrgID, &v.Plate, &v.Name, &v.DriverID, &v.Fuel, &v.TierID, &v.CustomLimit, &v.Extra, &v.Active, &v.TagBattery)
}

func (r pgRepo) ListVehicles(ctx context.Context, orgID string) ([]Vehicle, error) {
	rows, err := r.q.Query(ctx, `SELECT `+vehicleCols+` FROM vehicles WHERE org_id = $1 ORDER BY seq`, orgID)
	return many(rows, err, scanVehicle)
}

func (r pgRepo) GetVehicle(ctx context.Context, id string) (Vehicle, error) {
	return one(r.q.QueryRow(ctx, `SELECT `+vehicleCols+` FROM vehicles WHERE id = $1`+r.forUpdate(), id), scanVehicle)
}

func (r pgRepo) InsertVehicle(ctx context.Context, v Vehicle) (string, error) {
	var id string
	err := r.q.QueryRow(ctx, `INSERT INTO vehicles (org_id, plate, name, driver_id, fuel, tier_id, custom_limit, extra, active, tag_battery)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`,
		v.OrgID, v.Plate, v.Name, v.DriverID, v.Fuel, v.TierID, v.CustomLimit, v.Extra, v.Active, v.TagBattery).Scan(&id)
	return id, err
}

func (r pgRepo) UpdateVehicle(ctx context.Context, v Vehicle) error {
	_, err := r.q.Exec(ctx, `UPDATE vehicles SET plate = $2, name = $3, driver_id = $4, fuel = $5, tier_id = $6,
		custom_limit = $7, extra = $8, active = $9, tag_battery = $10 WHERE id = $1`,
		v.ID, v.Plate, v.Name, v.DriverID, v.Fuel, v.TierID, v.CustomLimit, v.Extra, v.Active, v.TagBattery)
	return err
}

// ---------- rules ----------

func (r pgRepo) GetRules(ctx context.Context, orgID string) (Rules, error) {
	return one(r.q.QueryRow(ctx, `SELECT network, anti_siphon, geofence, wrong_fuel, odometer, max_fill, max_fill_amount, hours, network_station_ids
		FROM rules WHERE org_id = $1`+r.forUpdate(), orgID),
		func(row pgx.Row) (Rules, error) {
			var x Rules
			return x, row.Scan(&x.Network, &x.AntiSiphon, &x.Geofence, &x.WrongFuel, &x.Odometer, &x.MaxFill, &x.MaxFillAmount, &x.Hours, &x.NetworkStationIDs)
		})
}

func (r pgRepo) UpdateRules(ctx context.Context, orgID string, x Rules) error {
	if x.NetworkStationIDs == nil {
		x.NetworkStationIDs = []string{}
	}
	_, err := r.q.Exec(ctx, `UPDATE rules SET network = $2, anti_siphon = $3, geofence = $4, wrong_fuel = $5, odometer = $6,
		max_fill = $7, max_fill_amount = $8, hours = $9, network_station_ids = $10 WHERE org_id = $1`,
		orgID, x.Network, x.AntiSiphon, x.Geofence, x.WrongFuel, x.Odometer, x.MaxFill, x.MaxFillAmount, x.Hours, x.NetworkStationIDs)
	return err
}

// ---------- requests ----------

const requestCols = `id, org_id, driver_id, vehicle_id, type, amount, reason, note, status, approved, reply, created_at, decided_at`

func scanRequest(row pgx.Row) (Request, error) {
	var q Request
	return q, row.Scan(&q.ID, &q.OrgID, &q.DriverID, &q.VehicleID, &q.Type, &q.Amount, &q.Reason, &q.Note, &q.Status, &q.Approved, &q.Reply, &q.CreatedAt, &q.DecidedAt)
}

func (r pgRepo) ListRequests(ctx context.Context, orgID string) ([]Request, error) {
	rows, err := r.q.Query(ctx, `SELECT `+requestCols+` FROM requests WHERE org_id = $1 ORDER BY created_at DESC, id DESC`, orgID)
	return many(rows, err, scanRequest)
}

func (r pgRepo) GetRequest(ctx context.Context, id string) (Request, error) {
	return one(r.q.QueryRow(ctx, `SELECT `+requestCols+` FROM requests WHERE id = $1`+r.forUpdate(), id), scanRequest)
}

func (r pgRepo) InsertRequest(ctx context.Context, q Request) (Request, error) {
	return one(r.q.QueryRow(ctx, `INSERT INTO requests (org_id, driver_id, vehicle_id, type, amount, reason, note, status, approved, reply)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING `+requestCols,
		q.OrgID, q.DriverID, q.VehicleID, q.Type, q.Amount, q.Reason, q.Note, q.Status, q.Approved, q.Reply), scanRequest)
}

func (r pgRepo) UpdateRequest(ctx context.Context, q Request) error {
	_, err := r.q.Exec(ctx, `UPDATE requests SET status = $2, approved = $3, reply = $4, decided_at = $5 WHERE id = $1`,
		q.ID, q.Status, q.Approved, q.Reply, q.DecidedAt)
	return err
}
