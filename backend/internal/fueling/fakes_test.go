package fueling

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"

	"github.com/bonbonati/fuel-management-hub/backend/internal/common"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/events"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/i18n"
)

// ---------- in-memory store ----------

type memStore struct{ st *memState }

type memState struct {
	seq        int
	txs        map[string]Transaction
	odo        []odoRow
	security   []SecurityEvent
	outbox     []events.Event
	failInsert bool // simulate a database failure after the charge
}

type odoRow struct {
	OdometerReading
	consumedBy string
}

func newMem() memStore {
	return memStore{st: &memState{seq: 90412, txs: map[string]Transaction{}}}
}

func (s *memState) clone() *memState {
	c := *s
	c.txs = maps.Clone(s.txs)
	c.odo, c.security, c.outbox = slices.Clone(s.odo), slices.Clone(s.security), slices.Clone(s.outbox)
	return &c
}

func (m memStore) InTx(_ context.Context, fn func(Repo) error) error {
	work := memStore{st: m.st.clone()}
	if err := fn(work); err != nil {
		return err
	}
	*m.st = *work.st
	return nil
}

func (m memStore) NextTxID(context.Context) (string, error) {
	m.st.seq++
	return fmt.Sprintf("T-%d", m.st.seq), nil
}
func (m memStore) InsertTx(_ context.Context, t Transaction) error {
	if m.st.failInsert {
		return errors.New("db down")
	}
	m.st.txs[t.ID] = t
	return nil
}
func (m memStore) GetTx(_ context.Context, id string) (Transaction, error) {
	t, ok := m.st.txs[id]
	if !ok {
		return t, ErrNotFound
	}
	return t, nil
}
func (m memStore) UpdateTx(_ context.Context, t Transaction) error { m.st.txs[t.ID] = t; return nil }
func (m memStore) ListTx(_ context.Context, f TxFilter, limit int) ([]Transaction, error) {
	var out []Transaction
	for _, t := range m.st.txs {
		if (f.DriverID != "" && t.DriverID == f.DriverID) || (f.OrgID != "" && t.OrgID == f.OrgID) || (f.StationID != "" && t.StationID == f.StationID) {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}
func (m memStore) FindOpenByCode(_ context.Context, station, code string) (Transaction, error) {
	for _, t := range m.st.txs {
		if t.StationID == station && t.Code == code && code != "" && t.Status == StatusAuthorized {
			return t, nil
		}
	}
	return Transaction{}, ErrNotFound
}
func (m memStore) CodeInUse(ctx context.Context, station, code string) (bool, error) {
	_, err := m.FindOpenByCode(ctx, station, code)
	return err == nil, nil
}
func (m memStore) PumpBusy(_ context.Context, station string, pump int) (bool, error) {
	for _, t := range m.st.txs {
		if t.StationID == station && t.Pump == pump && t.Status != StatusDone {
			return true, nil
		}
	}
	return false, nil
}
func (m memStore) PendingOdometer(_ context.Context, driver string) (*OdometerReading, error) {
	for i := len(m.st.odo) - 1; i >= 0; i-- {
		if r := m.st.odo[i]; r.DriverID == driver && r.consumedBy == "" {
			return &r.OdometerReading, nil
		}
	}
	return nil, nil
}
func (m memStore) InsertOdometer(_ context.Context, r OdometerReading) error {
	m.st.odo = append(m.st.odo, odoRow{OdometerReading: r})
	return nil
}
func (m memStore) ConsumeOdometer(_ context.Context, driver, tx string) error {
	for i := range m.st.odo {
		if m.st.odo[i].DriverID == driver && m.st.odo[i].consumedBy == "" {
			m.st.odo[i].consumedBy = tx
		}
	}
	return nil
}
func (m memStore) InsertSecurityEvent(_ context.Context, e SecurityEvent) error {
	m.st.security = append(m.st.security, e)
	return nil
}
func (m memStore) ListSecurityEvents(_ context.Context, org string, _ int) ([]SecurityEvent, error) {
	var out []SecurityEvent
	for _, e := range m.st.security {
		if e.OrgID == org {
			out = append(out, e)
		}
	}
	return out, nil
}
func (m memStore) Enqueue(_ context.Context, ev events.Event) error {
	m.st.outbox = append(m.st.outbox, ev)
	return nil
}

// ---------- fake upstream services ----------

// fakeFleet holds drivers/vehicles of org o1 (mirrors the demo seed).
type fakeFleet struct {
	drivers  map[string]*FleetDriver
	vehicles map[string]*FleetVehicle
	assign   map[string]string // vehicle → driver
	rules    FleetRules
}

func newFleet() *fakeFleet {
	return &fakeFleet{
		drivers: map[string]*FleetDriver{
			"d3": {ID: "d3", Name: i18n.T("عبدالرحمن حسن", "Abdurahman Hassan"), Code: "D-3071", Active: true},
			"d6": {ID: "d6", Name: i18n.T("حمد الأمين", "Hamad Alamin"), Code: "D-3117", Active: true},
			"d9": {ID: "d9", Name: i18n.T("بلا مركبة", "No Vehicle"), Code: "D-9", Active: true},
		},
		vehicles: map[string]*FleetVehicle{
			"f3": {ID: "f3", Name: i18n.T("هيونداي", "Hyundai H1"), Plate: common.Plate{Digits: "7781", Latin: "KRT", Arabic: "خ ر ط"}, Fuel: common.Fuel95, Active: true, Limit: 1_350_000},
			"f6": {ID: "f6", Name: i18n.T("كانتر", "Canter"), Plate: common.Plate{Digits: "9087", Latin: "BH", Arabic: "ب ح"}, Fuel: common.FuelDiesel, Active: true, Limit: 1_800_000},
		},
		assign: map[string]string{"f3": "d3", "f6": "d6"},
		rules:  FleetRules{Network: true, WrongFuel: true, Odometer: true, MaxFill: true, MaxFillAmount: 400_000, NetworkStationIDs: []string{"s1", "s2"}},
	}
}

func (f *fakeFleet) build(d *FleetDriver, v *FleetVehicle) FleetContext {
	c := FleetContext{OrgID: "o1", OrgName: i18n.T("النماء", "Al Namaa"), Rules: f.rules}
	if d != nil {
		dd := *d
		c.Driver = &dd
	}
	if v != nil {
		vv := *v
		c.Vehicle = &vv
	}
	switch {
	case d != nil && !d.Active:
		c.Block = &FleetBlock{Kind: "driver", Message: i18n.T("موقوف", "suspended")}
	case v == nil:
		c.Block = &FleetBlock{Kind: "noveh", Message: i18n.T("لا مركبة", "no vehicle")}
	case !v.Active:
		c.Block = &FleetBlock{Kind: "veh", Message: i18n.T("مركبة موقوفة", "vehicle suspended")}
	}
	return c
}

func (f *fakeFleet) DriverContext(_ context.Context, id string) (FleetContext, error) {
	d, ok := f.drivers[id]
	if !ok {
		return FleetContext{}, httpx.NotFound("not_found", "x", "x")
	}
	var v *FleetVehicle
	for vid, did := range f.assign {
		if did == id {
			v = f.vehicles[vid]
		}
	}
	return f.build(d, v), nil
}

func (f *fakeFleet) VehicleContext(_ context.Context, id string) (FleetContext, error) {
	v, ok := f.vehicles[id]
	if !ok {
		return FleetContext{}, httpx.NotFound("not_found", "x", "x")
	}
	return f.build(f.drivers[f.assign[id]], v), nil
}

type fakeStations struct{ pumps map[string]Pump }

func newStations() *fakeStations {
	return &fakeStations{pumps: map[string]Pump{
		"s1/3": {StationID: "s1", Number: 3, Grades: []common.Fuel{common.Fuel91, common.Fuel95}, Online: true},
		"s1/5": {StationID: "s1", Number: 5, Grades: common.Fuels, Online: true},
		"s1/7": {StationID: "s1", Number: 7, Grades: []common.Fuel{common.FuelDiesel}, Online: false},
		"s1/8": {StationID: "s1", Number: 8, Grades: []common.Fuel{common.FuelDiesel}, Online: true},
		"s3/1": {StationID: "s3", Number: 1, Grades: common.Fuels, Online: true},
	}}
}

func (f *fakeStations) Quote(_ context.Context, station string, n int) (PumpQuote, error) {
	p, ok := f.pumps[fmt.Sprintf("%s/%d", station, n)]
	if !ok {
		return PumpQuote{}, httpx.NotFound("not_found", "x", "x")
	}
	return PumpQuote{Pump: p, Prices: common.DefaultPrices}, nil
}

func (f *fakeStations) Station(_ context.Context, id string) (StationInfo, error) {
	return StationInfo{ID: id, Name: i18n.T("محطة النيل الأزرق", "Blue Nile Station")}, nil
}

// fakePayments keeps a balance and per-vehicle usage and enforces the same
// limit/balance rules as the payment service.
type fakePayments struct {
	balance  int64
	used     map[string]int64
	charges  map[string]ChargeRequest
	refunds  []string
	failNext error
}

func newPayments() *fakePayments {
	return &fakePayments{balance: 18_450_000, used: map[string]int64{"f3": 768_000, "f6": 1_440_000}, charges: map[string]ChargeRequest{}}
}

func (p *fakePayments) Allowance(_ context.Context, _, veh string) (Allowance, error) {
	return Allowance{Balance: p.balance, Used: p.used[veh]}, nil
}

func (p *fakePayments) Charge(_ context.Context, in ChargeRequest) error {
	if err := p.failNext; err != nil {
		p.failNext = nil
		return err
	}
	if p.used[in.VehicleID]+in.Amount > in.VehicleLimit {
		return httpx.Conflict("limit_exceeded", "x", "x")
	}
	p.charges[in.TransactionID] = in
	p.balance -= in.Amount
	p.used[in.VehicleID] += in.Amount
	return nil
}

func (p *fakePayments) Refund(_ context.Context, tx string) error {
	c := p.charges[tx]
	p.balance += c.Amount
	p.used[c.VehicleID] -= c.Amount
	p.refunds = append(p.refunds, tx)
	return nil
}

var fixedNow = time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
