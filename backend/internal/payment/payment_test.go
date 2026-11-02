package payment

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn/authntest"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/events"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
)

// memStore is an in-memory Store; InTx commits a copy only on success.
type memStore struct{ st *memState }

type memState struct {
	wallets     map[string]int64
	ledger      []Entry
	tax         Tax
	settlements map[string]Settlement // key station|date
	processed   map[string]bool
}

func (s *memState) clone() *memState {
	c := *s
	c.wallets, c.settlements, c.processed = maps.Clone(s.wallets), maps.Clone(s.settlements), maps.Clone(s.processed)
	c.ledger = slices.Clone(s.ledger)
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

func (m memStore) LockWallet(_ context.Context, org string) (int64, error) {
	b, ok := m.st.wallets[org]
	if !ok {
		return 0, ErrNotFound
	}
	return b, nil
}
func (m memStore) SetBalance(_ context.Context, org string, b int64) error {
	m.st.wallets[org] = b
	return nil
}
func (m memStore) FindEntry(_ context.Context, kind, tx string) (Entry, error) {
	for _, e := range m.st.ledger {
		if e.Kind == kind && e.TxID != nil && *e.TxID == tx {
			return e, nil
		}
	}
	return Entry{}, ErrNotFound
}
func (m memStore) InsertEntry(_ context.Context, e Entry) error {
	m.st.ledger = append(m.st.ledger, e)
	return nil
}
func (m memStore) usage(org string, since time.Time) map[string]int64 {
	out := map[string]int64{}
	for _, e := range m.st.ledger {
		if e.OrgID != org || e.VehicleID == nil || e.CreatedAt.Before(since) {
			continue
		}
		switch e.Kind {
		case KindOpening, KindCharge:
			out[*e.VehicleID] += e.Amount
		case KindRefund:
			out[*e.VehicleID] -= e.Amount
		}
	}
	return out
}
func (m memStore) VehicleUsage(_ context.Context, org, veh string, since time.Time) (int64, error) {
	return m.usage(org, since)[veh], nil
}
func (m memStore) OrgUsage(_ context.Context, org string, since time.Time) (map[string]int64, error) {
	return m.usage(org, since), nil
}
func (m memStore) GetTax(context.Context) (Tax, error)   { return m.st.tax, nil }
func (m memStore) SetTax(_ context.Context, t Tax) error { m.st.tax = t; return nil }
func (m memStore) ListSettlements(_ context.Context, station string, limit int) ([]Settlement, error) {
	var out []Settlement
	for _, s := range m.st.settlements {
		if s.StationID == station {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (m memStore) AccrueSettlement(_ context.Context, station, date string, amount, fee int64) error {
	k := station + "|" + date
	s, ok := m.st.settlements[k]
	if !ok {
		s = Settlement{StationID: station, Date: date, Status: "scheduled"}
	}
	s.Amount += amount
	s.Fee += fee
	m.st.settlements[k] = s
	return nil
}
func (m memStore) MarkProcessed(_ context.Context, id string) (bool, error) {
	if m.st.processed[id] {
		return false, nil
	}
	m.st.processed[id] = true
	return true, nil
}

// fixed "now" for deterministic month boundaries: 2 Nov 2026, 10:00 Khartoum.
var now = time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)

func newSvc() (*Service, memStore) {
	st := memStore{st: &memState{
		wallets: map[string]int64{"o1": 1_000_000}, tax: Tax{Enabled: true, Rate: 17},
		settlements: map[string]Settlement{}, processed: map[string]bool{},
	}}
	f3, op := "f3", "opening-f3"
	st.st.ledger = []Entry{
		{OrgID: "o1", VehicleID: &f3, TxID: &op, Kind: KindOpening, Amount: 768_000, CreatedAt: now.Add(-24 * time.Hour)},
		// Last month's charge must not count towards this month's usage.
		{OrgID: "o1", VehicleID: &f3, Kind: KindCharge, Amount: 999_999, CreatedAt: time.Date(2026, 10, 31, 20, 0, 0, 0, time.UTC)},
	}
	svc := NewService(st, 43)
	svc.now = func() time.Time { return now }
	return svc, st
}

var ctx = context.Background()

func code(err error) string {
	if e := httpx.AsError(err); e != nil {
		return e.Code
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func TestCheckCharge(t *testing.T) {
	cases := []struct {
		name                         string
		balance, used, limit, amount int64
		want                         string
	}{
		{"fits", 1_000, 100, 500, 400, ""},
		{"exactly the remaining limit", 1_000, 100, 500, 400, ""},
		{"exactly the balance", 400, 0, 1_000, 400, ""},
		{"over limit", 1_000, 100, 500, 401, "limit_exceeded"},
		{"over balance", 300, 0, 1_000, 400, "insufficient_balance"},
		{"limit reported before balance", 10, 0, 5, 50, "limit_exceeded"},
		{"zero amount", 1_000, 0, 1_000, 0, "invalid_amount"},
		{"negative amount", 1_000, 0, 1_000, -5, "invalid_amount"},
	}
	for _, tc := range cases {
		if got := code(CheckCharge(tc.balance, tc.used, tc.limit, tc.amount)); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestChargeIsAtomicAndIdempotent(t *testing.T) {
	svc, st := newSvc()
	in := ChargeInput{TransactionID: "T-1", OrgID: "o1", VehicleID: "f3", Amount: 200_000, VehicleLimit: 1_350_000}
	res, err := svc.Charge(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Balance != 800_000 || res.Used != 968_000 || res.Replayed {
		t.Fatalf("result = %+v (last month's charge must be excluded)", res)
	}
	// Replay: no double charge.
	res2, err := svc.Charge(ctx, in)
	if err != nil || !res2.Replayed || st.st.wallets["o1"] != 800_000 {
		t.Fatalf("replay = %+v, %v, balance %d", res2, err, st.st.wallets["o1"])
	}
	// Over the remaining limit: 1,350,000 - 968,000 = 382,000.
	_, err = svc.Charge(ctx, ChargeInput{TransactionID: "T-2", OrgID: "o1", VehicleID: "f3", Amount: 382_001, VehicleLimit: 1_350_000})
	if code(err) != "limit_exceeded" {
		t.Fatalf("code = %q", code(err))
	}
	// Over the balance.
	_, err = svc.Charge(ctx, ChargeInput{TransactionID: "T-3", OrgID: "o1", VehicleID: "f9", Amount: 900_000, VehicleLimit: 5_000_000})
	if code(err) != "insufficient_balance" {
		t.Fatalf("code = %q", code(err))
	}
	if st.st.wallets["o1"] != 800_000 || len(st.st.ledger) != 3 {
		t.Fatal("failed charges must not write anything")
	}
	if _, err := svc.Charge(ctx, ChargeInput{TransactionID: "T-4", OrgID: "nope", VehicleID: "f", Amount: 1, VehicleLimit: 1}); code(err) != "no_wallet" {
		t.Fatalf("unknown org code = %q", code(err))
	}
}

func TestRefund(t *testing.T) {
	svc, st := newSvc()
	_, _ = svc.Charge(ctx, ChargeInput{TransactionID: "T-1", OrgID: "o1", VehicleID: "f3", Amount: 100_000, VehicleLimit: 2_000_000})
	for range 2 { // idempotent
		if err := svc.Refund(ctx, "T-1"); err != nil {
			t.Fatal(err)
		}
	}
	if st.st.wallets["o1"] != 1_000_000 {
		t.Fatalf("balance = %d after refund", st.st.wallets["o1"])
	}
	if al, _ := svc.Allowance(ctx, "o1", "f3"); al.Used != 768_000 {
		t.Fatalf("usage after refund = %d", al.Used)
	}
	if err := svc.Refund(ctx, "T-404"); code(err) != "no_charge" {
		t.Fatalf("code = %q", code(err))
	}
}

func TestWalletAndTopUp(t *testing.T) {
	svc, _ := newSvc()
	w, err := svc.TopUp(ctx, "o1", 5_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if w.Balance != 6_000_000 || w.Month != "2026-11" || w.Usage["f3"] != 768_000 {
		t.Fatalf("wallet = %+v", w)
	}
	for _, bad := range []int64{0, 9_999, 1_000_000_001} {
		if _, err := svc.TopUp(ctx, "o1", bad); code(err) != "invalid_amount" {
			t.Errorf("top-up %d: code %q", bad, code(err))
		}
	}
}

func TestSetTax(t *testing.T) {
	svc, st := newSvc()
	if _, err := svc.SetTax(ctx, Tax{Enabled: true, Rate: 15.5}); err != nil || st.st.tax.Rate != 15.5 {
		t.Fatalf("set tax: %v %+v", err, st.st.tax)
	}
	for _, bad := range []float64{-1, 100.5} {
		if _, err := svc.SetTax(ctx, Tax{Enabled: true, Rate: bad}); code(err) != "invalid_rate" {
			t.Errorf("rate %v accepted", bad)
		}
	}
}

func TestAccrueCompletedIdempotentAndFee(t *testing.T) {
	svc, st := newSvc()
	completed := time.Date(2026, 11, 2, 21, 30, 0, 0, time.UTC) // 23:30 Khartoum (UTC+2)
	for range 2 {
		if err := svc.AccrueCompleted(ctx, "ev-1", "s1", 200_000, completed); err != nil {
			t.Fatal(err)
		}
	}
	s := st.st.settlements["s1|2026-11-03"]
	if s.Amount != 200_000 || s.Fee != 860 || s.Status != "scheduled" {
		t.Fatalf("settlement = %+v (duplicate event must be ignored; fee 0.43%%)", s)
	}
	// Just after midnight Khartoum the payout moves to the following day.
	_ = svc.AccrueCompleted(ctx, "ev-2", "s1", 100_000, completed.Add(time.Hour))
	if st.st.settlements["s1|2026-11-04"].Amount != 100_000 {
		t.Fatalf("payout date not in Khartoum time: %+v", st.st.settlements)
	}
	if err := svc.AccrueCompleted(ctx, "ev-3", "", 100, completed); err != nil || len(st.st.processed) != 2 {
		t.Fatal("events without a station must be ignored")
	}
}

func TestSettlementsSummary(t *testing.T) {
	svc, st := newSvc()
	st.st.settlements = map[string]Settlement{
		"a": {StationID: "s1", Date: "2026-11-03", Amount: 10, Fee: 1, Status: "scheduled"},
		"b": {StationID: "s1", Date: "2026-11-02", Amount: 20, Fee: 2, Status: "paid"},
		"c": {StationID: "s1", Date: "2026-10-31", Amount: 30, Fee: 4, Status: "paid"},
		"d": {StationID: "s2", Date: "2026-11-03", Amount: 99, Fee: 9, Status: "scheduled"},
	}
	sum, err := svc.Settlements(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Items) != 3 || sum.Items[0].Date != "2026-11-03" || sum.FeesThisMonth != 3 || sum.NextPayout == nil || sum.NextPayout.Amount != 10 {
		t.Fatalf("summary = %+v", sum)
	}
}

func TestSettlementHandlerDecodesEvent(t *testing.T) {
	svc, st := newSvc()
	ev, _ := events.New("fueling", events.TypeTransactionCompleted, events.TransactionCompleted{TransactionID: "T-1", StationID: "s1", Amount: 50_000, CompletedAt: now})
	if err := svc.SettlementHandler(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if st.st.settlements["s1|2026-11-03"].Amount != 50_000 {
		t.Fatalf("settlements = %+v", st.st.settlements)
	}
	bad := events.Event{ID: "x", Type: "t", Data: json.RawMessage(`"oops"`)}
	if err := svc.SettlementHandler(ctx, bad); err != nil {
		t.Fatal("malformed payloads must be acknowledged, not retried")
	}
}

func TestHTTPGuards(t *testing.T) {
	env := authntest.New(t)
	svc, _ := newSvc()
	c := env.Server(NewAPI(svc).Register)
	fm := env.Token(authn.RoleFleetManager, "o1", "", "m1")
	drv := env.Token(authn.RoleDriver, "o1", "", "d3")
	otherDrv := env.Token(authn.RoleDriver, "o2", "", "dx")
	sa := env.Token(authn.RoleStationAdmin, "", "s1", "a1")

	cases := []struct {
		method, path, tok string
		body              any
		want              int
	}{
		{"GET", "/api/v1/payments/wallet", drv, nil, 403},
		{"GET", "/api/v1/payments/wallet", fm, nil, 200},
		{"POST", "/api/v1/payments/wallet/topups", fm, map[string]int64{"amount": 5_000_000}, 200},
		{"GET", "/api/v1/payments/tax", drv, nil, 200},
		{"PUT", "/api/v1/payments/tax", drv, Tax{Enabled: false}, 403},
		{"PUT", "/api/v1/payments/tax", sa, Tax{Enabled: true, Rate: 17}, 200},
		{"GET", "/api/v1/payments/settlements", sa, nil, 200},
		{"GET", "/api/v1/payments/settlements", fm, nil, 403},
		{"GET", "/internal/v1/payments/allowance?orgId=o1&vehicleId=f3", drv, nil, 200},
		{"GET", "/internal/v1/payments/allowance?orgId=o1&vehicleId=f3", otherDrv, nil, 403},
		{"POST", "/internal/v1/payments/charges", otherDrv, ChargeInput{TransactionID: "T", OrgID: "o1", VehicleID: "f3", Amount: 1, VehicleLimit: 9_999_999}, 403},
		{"POST", "/internal/v1/payments/charges", fm, ChargeInput{TransactionID: "T", OrgID: "o1", VehicleID: "f3", Amount: 1, VehicleLimit: 9_999_999}, 403},
		{"POST", "/internal/v1/payments/charges", drv, ChargeInput{TransactionID: "T", OrgID: "o1", VehicleID: "f3", Amount: 10_000, VehicleLimit: 9_999_999}, 200},
	}
	for _, tc := range cases {
		if got := c.Do(tc.method, tc.path, tc.tok, tc.body, nil); got != tc.want {
			t.Errorf("%s %s: %d, want %d", tc.method, tc.path, got, tc.want)
		}
	}
	var w Wallet
	if c.Do("GET", "/api/v1/payments/wallet", fm, nil, &w); w.Balance != 5_990_000 {
		t.Fatalf("balance = %d", w.Balance)
	}
	if s := c.Do("POST", "/internal/v1/payments/refunds", drv, map[string]string{"transactionId": "T"}, nil); s != http.StatusOK {
		t.Fatalf("refund status %d", s)
	}
}
