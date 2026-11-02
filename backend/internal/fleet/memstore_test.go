package fleet

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"

	"github.com/bonbonati/fuel-management-hub/backend/internal/common"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/i18n"
)

// memStore is an in-memory Store used by the unit tests. InTx works on a copy
// of the state and only commits it when fn succeeds, so tests can assert that
// failed use cases leave no partial writes (same guarantee as Postgres).
type memStore struct{ st *memState }

type memState struct {
	orgs     map[string]Org
	tiers    map[string]Tier
	drivers  map[string]Driver
	vehicles map[string]Vehicle
	rules    map[string]Rules
	requests map[string]Request
	order    map[string]int // insertion order, mirrors the seq column
	seq      int
	reqSeq   int
}

func (s *memState) clone() *memState {
	c := *s
	c.orgs, c.tiers, c.drivers = maps.Clone(s.orgs), maps.Clone(s.tiers), maps.Clone(s.drivers)
	c.vehicles, c.rules, c.requests, c.order = maps.Clone(s.vehicles), maps.Clone(s.rules), maps.Clone(s.requests), maps.Clone(s.order)
	return &c
}

func (s *memState) add(id string) {
	s.seq++
	s.order[id] = s.seq
}

func ordered[T any](m map[string]T, s *memState, keep func(T) bool) []T {
	ids := slices.Collect(maps.Keys(m))
	sort.Slice(ids, func(i, j int) bool { return s.order[ids[i]] < s.order[ids[j]] })
	var out []T
	for _, id := range ids {
		if keep(m[id]) {
			out = append(out, m[id])
		}
	}
	return out
}

func get[T any](m map[string]T, id string) (T, error) {
	v, ok := m[id]
	if !ok {
		return v, ErrNotFound
	}
	return v, nil
}

func (m memStore) InTx(_ context.Context, fn func(Repo) error) error {
	work := memStore{st: m.st.clone()}
	if err := fn(work); err != nil {
		return err
	}
	*m.st = *work.st
	return nil
}

func (m memStore) GetOrg(_ context.Context, id string) (Org, error) { return get(m.st.orgs, id) }
func (m memStore) UpdateOrg(_ context.Context, o Org) error         { m.st.orgs[o.ID] = o; return nil }

func (m memStore) ListTiers(_ context.Context, org string) ([]Tier, error) {
	return ordered(m.st.tiers, m.st, func(t Tier) bool { return t.OrgID == org }), nil
}
func (m memStore) GetTier(_ context.Context, id string) (Tier, error) { return get(m.st.tiers, id) }
func (m memStore) InsertTier(_ context.Context, t Tier) (string, error) {
	t.ID = fmt.Sprintf("t%d", 100+m.st.seq)
	m.st.tiers[t.ID] = t
	m.st.add(t.ID)
	return t.ID, nil
}
func (m memStore) UpdateTier(_ context.Context, t Tier) error { m.st.tiers[t.ID] = t; return nil }

func (m memStore) ListDrivers(_ context.Context, org string) ([]Driver, error) {
	return ordered(m.st.drivers, m.st, func(d Driver) bool { return d.OrgID == org }), nil
}
func (m memStore) GetDriver(_ context.Context, id string) (Driver, error) {
	return get(m.st.drivers, id)
}
func (m memStore) InsertDriver(_ context.Context, d Driver) (string, error) {
	if m.codeTaken(d) {
		return "", errDuplicateCode
	}
	d.ID = fmt.Sprintf("d%d", 100+m.st.seq)
	m.st.drivers[d.ID] = d
	m.st.add(d.ID)
	return d.ID, nil
}
func (m memStore) UpdateDriver(_ context.Context, d Driver) error {
	if m.codeTaken(d) {
		return errDuplicateCode
	}
	m.st.drivers[d.ID] = d
	return nil
}
func (m memStore) codeTaken(d Driver) bool {
	for _, x := range m.st.drivers {
		if x.ID != d.ID && x.OrgID == d.OrgID && x.Code == d.Code {
			return true
		}
	}
	return false
}

func (m memStore) ListVehicles(_ context.Context, org string) ([]Vehicle, error) {
	return ordered(m.st.vehicles, m.st, func(v Vehicle) bool { return v.OrgID == org }), nil
}
func (m memStore) GetVehicle(_ context.Context, id string) (Vehicle, error) {
	return get(m.st.vehicles, id)
}
func (m memStore) InsertVehicle(ctx context.Context, v Vehicle) (string, error) {
	v.ID = fmt.Sprintf("f%d", 100+m.st.seq)
	if err := m.checkDriverUnique(v); err != nil {
		return "", err
	}
	m.st.vehicles[v.ID] = v
	m.st.add(v.ID)
	return v.ID, nil
}
func (m memStore) UpdateVehicle(_ context.Context, v Vehicle) error {
	if err := m.checkDriverUnique(v); err != nil {
		return err
	}
	m.st.vehicles[v.ID] = v
	return nil
}

// checkDriverUnique mirrors the vehicles_driver_key unique index.
func (m memStore) checkDriverUnique(v Vehicle) error {
	if v.DriverID == nil {
		return nil
	}
	for _, x := range m.st.vehicles {
		if x.ID != v.ID && x.DriverID != nil && *x.DriverID == *v.DriverID {
			return fmt.Errorf("unique violation: driver %s already on %s", *v.DriverID, x.ID)
		}
	}
	return nil
}

func (m memStore) GetRules(_ context.Context, org string) (Rules, error) { return get(m.st.rules, org) }
func (m memStore) UpdateRules(_ context.Context, org string, r Rules) error {
	m.st.rules[org] = r
	return nil
}

func (m memStore) ListRequests(_ context.Context, org string) ([]Request, error) {
	out := ordered(m.st.requests, m.st, func(r Request) bool { return r.OrgID == org })
	slices.Reverse(out) // newest first
	return out, nil
}
func (m memStore) GetRequest(_ context.Context, id string) (Request, error) {
	return get(m.st.requests, id)
}
func (m memStore) InsertRequest(_ context.Context, r Request) (Request, error) {
	m.st.reqSeq++
	r.ID = fmt.Sprintf("R-%d", 1043+m.st.reqSeq)
	r.CreatedAt = time.Now()
	m.st.requests[r.ID] = r
	m.st.add(r.ID)
	return r, nil
}
func (m memStore) UpdateRequest(_ context.Context, r Request) error {
	m.st.requests[r.ID] = r
	return nil
}

func ptr[T any](v T) *T { return &v }

// newSeededStore returns a memStore holding the same demo data as seed.sql
// (subset sufficient for the tests) plus a second org for isolation tests.
func newSeededStore() memStore {
	st := &memState{
		orgs: map[string]Org{}, tiers: map[string]Tier{}, drivers: map[string]Driver{},
		vehicles: map[string]Vehicle{}, rules: map[string]Rules{}, requests: map[string]Request{}, order: map[string]int{},
	}
	put := func(id string, f func()) { f(); st.add(id) }
	put("o1", func() {
		st.orgs["o1"] = Org{ID: "o1", Name: i18n.T("شركة النماء", "Al Namaa"), Phone: "+249 91 230 4471", City: i18n.T("الخرطوم", "Khartoum")}
	})
	put("o2", func() { st.orgs["o2"] = Org{ID: "o2", Name: i18n.T("أخرى", "Other Co"), Phone: "+249 1"} })
	st.rules["o1"] = Rules{Network: true, WrongFuel: true, Odometer: true, MaxFill: true, MaxFillAmount: 400000, NetworkStationIDs: []string{"s1", "s2"}}
	st.rules["o2"] = Rules{}
	put("t1", func() {
		st.tiers["t1"] = Tier{ID: "t1", OrgID: "o1", Icon: "truck", Color: "or", Name: i18n.T("شاحنات", "Trucks"), Budget: 1500000, DailyLitres: 200, Window: "24/7"}
	})
	put("t2", func() {
		st.tiers["t2"] = Tier{ID: "t2", OrgID: "o1", Icon: "bus", Color: "bl", Name: i18n.T("حافلات", "Vans"), Budget: 1200000, DailyLitres: 80, Window: "06:00–21:00"}
	})
	put("tx", func() {
		st.tiers["tx"] = Tier{ID: "tx", OrgID: "o2", Name: i18n.T("أ", "A"), Budget: 10, DailyLitres: 1, Window: "x"}
	})
	for _, d := range []Driver{
		{ID: "d1", OrgID: "o1", Name: i18n.T("فيصل", "Faisal"), Phone: "+249 91 234 2114", Code: "D-3012", AppActivated: true, Active: true},
		{ID: "d3", OrgID: "o1", Name: i18n.T("عبدالرحمن", "Abdurahman"), Phone: "+249 90 552 4471", Code: "D-3071", AppActivated: true, Active: true},
		{ID: "d4", OrgID: "o1", Name: i18n.T("سعد", "Saad"), Phone: "+249 99 407 0952", Code: "D-3088", AppActivated: true, Active: true},
		{ID: "dx", OrgID: "o2", Name: i18n.T("س", "X"), Phone: "+249 99 000 0000", Code: "X-1", Active: true},
	} {
		put(d.ID, func() { st.drivers[d.ID] = d })
	}
	for _, v := range []Vehicle{
		{ID: "f1", OrgID: "o1", Plate: common.Plate{Digits: "4410", Latin: "KRT", Arabic: "خ ر ط"}, Name: i18n.T("إيسوزو", "Isuzu"), DriverID: ptr("d1"), Fuel: common.FuelDiesel, TierID: ptr("t1"), Active: true, TagBattery: 92},
		{ID: "f3", OrgID: "o1", Plate: common.Plate{Digits: "7781", Latin: "KRT", Arabic: "خ ر ط"}, Name: i18n.T("هيونداي", "Hyundai"), DriverID: ptr("d3"), Fuel: common.Fuel95, TierID: ptr("t2"), Extra: 150000, Active: true, TagBattery: 88},
		{ID: "f6", OrgID: "o1", Plate: common.Plate{Digits: "9087", Latin: "BH", Arabic: "ب ح"}, Name: i18n.T("كانتر", "Canter"), Fuel: common.FuelDiesel, TierID: ptr("t1"), CustomLimit: ptr(int64(1800000)), Active: true, TagBattery: 95},
		{ID: "fx", OrgID: "o2", Plate: common.Plate{Digits: "1", Latin: "K"}, Name: i18n.T("س", "X"), DriverID: ptr("dx"), Fuel: common.Fuel91, Active: true},
	} {
		put(v.ID, func() { st.vehicles[v.ID] = v })
	}
	put("R-1043", func() {
		st.requests["R-1043"] = Request{ID: "R-1043", OrgID: "o1", DriverID: "d4", Type: RequestLimit, Amount: 200000, Reason: "trip", Status: StatusPending}
	})
	return memStore{st: st}
}
