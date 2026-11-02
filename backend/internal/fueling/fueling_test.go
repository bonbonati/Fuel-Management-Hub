package fueling

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/bonbonati/fuel-management-hub/backend/internal/common"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn/authntest"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/events"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/kv"
)

var ctx = context.Background()

type fx struct {
	svc      *Service
	store    memStore
	kv       *kv.Memory
	fleet    *fakeFleet
	stations *fakeStations
	pay      *fakePayments
}

func newFx() fx {
	f := fx{store: newMem(), kv: kv.NewMemory(), fleet: newFleet(), stations: newStations(), pay: newPayments()}
	f.svc = NewService(f.store, f.kv, f.fleet, f.stations, f.pay)
	f.svc.now = func() time.Time { return fixedNow }
	return f
}

var (
	attendant = &authn.Claims{Role: authn.RoleAttendant, Station: "s1", Actor: "a2"}
	otherStn  = &authn.Claims{Role: authn.RoleAttendant, Station: "s2", Actor: "b1"}
)

func code(err error) string {
	if e := httpx.AsError(err); e != nil {
		return e.Code
	}
	if err != nil {
		return "untyped: " + err.Error()
	}
	return ""
}

// ---------- pure rules ----------

func TestCap(t *testing.T) {
	maxFill := FleetRules{MaxFill: true, MaxFillAmount: 400_000}
	cases := []struct {
		name                 string
		limit, used, balance int64
		r                    FleetRules
		want                 int64
		reason               string
	}{
		{"remaining limit binds", 1_350_000, 1_200_000, 9_000_000, FleetRules{}, 150_000, ReasonLimit},
		{"balance binds", 1_350_000, 0, 50_000, FleetRules{}, 50_000, ReasonLimit},
		{"per-fill rule binds", 1_350_000, 0, 9_000_000, maxFill, 400_000, ReasonPerFill},
		{"per-fill ignored when larger than remaining", 1_350_000, 1_200_000, 9_000_000, maxFill, 150_000, ReasonLimit},
		{"per-fill off", 1_350_000, 0, 9_000_000, FleetRules{MaxFillAmount: 1}, 1_350_000, ReasonLimit},
		{"over limit never negative", 100, 500, 9_000_000, FleetRules{}, 0, ReasonLimit},
	}
	for _, tc := range cases {
		if got, why := Cap(tc.limit, tc.used, tc.balance, tc.r); got != tc.want || why != tc.reason {
			t.Errorf("%s: Cap = %d/%s, want %d/%s", tc.name, got, why, tc.want, tc.reason)
		}
	}
}

func TestInNetworkAndOdometerParsing(t *testing.T) {
	r := FleetRules{Network: true, NetworkStationIDs: []string{"s1"}}
	if !InNetwork(r, "s1") || InNetwork(r, "s3") || !InNetwork(FleetRules{}, "s3") {
		t.Fatal("InNetwork misbehaves")
	}
	for in, ok := range map[string]bool{"142880": true, " 999 ": true, "12": false, "12345678": false, "14a880": false, "": false} {
		if _, err := ParseOdometer(in); (err == nil) != ok {
			t.Errorf("ParseOdometer(%q) err = %v", in, err)
		}
	}
}

func TestTokensAndCodes(t *testing.T) {
	tok := NewPassToken("D-3071", common.FuelDiesel)
	if !regexp.MustCompile(`^PF-3071-DZ-[0-9A-F]{8}$`).MatchString(tok) {
		t.Fatalf("token %q", tok)
	}
	if NewPassToken("D-3071", common.Fuel95) == NewPassToken("D-3071", common.Fuel95) {
		t.Fatal("tokens must be random")
	}
	for range 200 {
		n, err := strconv.Atoi(StartCode())
		if err != nil || n < 1000 || n > 9999 {
			t.Fatalf("start code %d out of range", n)
		}
	}
}

func TestFormatSDG(t *testing.T) {
	for n, want := range map[int64]string{0: "0 SDG", 999: "999 SDG", 1000: "1,000 SDG", 400000: "400,000 SDG", 18450000: "18,450,000 SDG", -1500: "-1,500 SDG"} {
		if got := FormatSDG(n).En; got != want {
			t.Errorf("FormatSDG(%d) = %q, want %q", n, got, want)
		}
	}
	if FormatSDG(1000).Ar != "1,000 ج.س" {
		t.Fatalf("arabic = %q", FormatSDG(1000).Ar)
	}
}

// ---------- driver flow ----------

func TestPassStatusAndOdometer(t *testing.T) {
	f := newFx()
	st, err := f.svc.PassStatus(ctx, "d3")
	if err != nil {
		t.Fatal(err)
	}
	a := st.Allowance
	if a == nil || a.Limit != 1_350_000 || a.Used != 768_000 || a.Remaining != 582_000 || a.Available != 400_000 || a.Reason != ReasonPerFill {
		t.Fatalf("allowance = %+v", a)
	}
	if !st.Odometer.Required || st.Odometer.Recorded {
		t.Fatalf("odometer = %+v", st.Odometer)
	}
	if _, err := f.svc.RecordOdometer(ctx, "d3", "14x"); code(err) != "invalid_odometer" {
		t.Fatalf("bad reading: %q", code(err))
	}
	if _, err := f.svc.RecordOdometer(ctx, "d3", "142880"); err != nil {
		t.Fatal(err)
	}
	st, _ = f.svc.PassStatus(ctx, "d3")
	if !st.Odometer.Recorded || *st.Odometer.Km != 142880 {
		t.Fatalf("odometer after record = %+v", st.Odometer)
	}
	if st, _ := f.svc.PassStatus(ctx, "d9"); st.Allowance != nil || st.Block == nil || st.Block.Kind != "noveh" {
		t.Fatalf("driver without vehicle = %+v", st)
	}
}

func TestIssuePass(t *testing.T) {
	f := newFx()
	p, err := f.svc.IssuePass(ctx, "d3")
	if err != nil {
		t.Fatal(err)
	}
	if p.TTLSeconds != 30 || !p.ExpiresAt.Equal(fixedNow.Add(30*time.Second)) {
		t.Fatalf("pass = %+v", p)
	}
	var claim passClaim
	if found, _ := f.kv.GetJSON(ctx, "pass:"+p.Token, &claim); !found || claim.VehicleID != "f3" || claim.DriverID != "d3" {
		t.Fatalf("claim = %+v", claim)
	}
	if _, err := f.svc.IssuePass(ctx, "d9"); code(err) != "no_vehicle" {
		t.Fatalf("no vehicle: %q", code(err))
	}
}

func authInput(amount int64) AuthorizeInput {
	return AuthorizeInput{StationID: "s1", Pump: 3, Fuel: common.Fuel95, Amount: amount}
}

func TestAuthorizeHappyPathConsumesOdometer(t *testing.T) {
	f := newFx()
	if _, err := f.svc.Authorize(ctx, "d3", authInput(200_000)); code(err) != "odometer_required" {
		t.Fatalf("without odometer: %q", code(err))
	}
	_, _ = f.svc.RecordOdometer(ctx, "d3", "142880")
	tx, err := f.svc.Authorize(ctx, "d3", authInput(200_000))
	if err != nil {
		t.Fatal(err)
	}
	if tx.Status != StatusAuthorized || tx.Source != SourceApp || len(tx.Code) != 4 || tx.OrgID != "o1" || tx.VehicleID != "f3" ||
		tx.PricePerLitre != 3600 || tx.OdometerKm == nil || *tx.OdometerKm != 142880 || tx.Plate.Digits != "7781" {
		t.Fatalf("tx = %+v", tx)
	}
	if tx.Litres < 55.5 || tx.Litres > 55.6 {
		t.Fatalf("litres = %v, want 200000/3600", tx.Litres)
	}
	if c := f.pay.charges[tx.ID]; c.Amount != 200_000 || c.VehicleLimit != 1_350_000 {
		t.Fatalf("charge = %+v", c)
	}
	if _, ok := f.store.st.txs[tx.ID]; !ok {
		t.Fatal("transaction not stored")
	}
	// The reading was used; the next fill-up needs a new one.
	if _, err := f.svc.Authorize(ctx, "d3", authInput(100_000)); code(err) != "odometer_required" {
		t.Fatalf("second fill-up: %q", code(err))
	}
}

func TestAuthorizeRejections(t *testing.T) {
	cases := []struct {
		name  string
		setup func(f fx)
		in    AuthorizeInput
		want  string
	}{
		{"suspended driver", func(f fx) { f.fleet.drivers["d3"].Active = false }, authInput(100_000), "blocked"},
		{"suspended vehicle", func(f fx) { f.fleet.vehicles["f3"].Active = false }, authInput(100_000), "blocked"},
		{"station outside network", nil, AuthorizeInput{StationID: "s3", Pump: 1, Fuel: common.Fuel95, Amount: 100_000}, "station_not_in_network"},
		{"wrong fuel", nil, AuthorizeInput{StationID: "s1", Pump: 3, Fuel: common.Fuel91, Amount: 100_000}, "wrong_fuel"},
		{"unknown fuel", nil, AuthorizeInput{StationID: "s1", Pump: 3, Fuel: "98", Amount: 100_000}, "invalid_fuel"},
		{"below minimum", nil, authInput(9_999), "invalid_amount"},
		{"pump lacks grade", nil, AuthorizeInput{StationID: "s1", Pump: 8, Fuel: common.Fuel95, Amount: 100_000}, "pump_unavailable"},
		{"unknown pump", nil, AuthorizeInput{StationID: "s1", Pump: 99, Fuel: common.Fuel95, Amount: 100_000}, "not_found"},
		{"over per-fill cap", nil, authInput(400_001), "exceeds_per_fill"},
		{"over remaining limit", func(f fx) { f.pay.used["f3"] = 1_300_000 }, authInput(60_000), "exceeds_available"},
		{"wrong fuel allowed when rule off", func(f fx) { f.fleet.rules.WrongFuel = false }, AuthorizeInput{StationID: "s1", Pump: 3, Fuel: common.Fuel91, Amount: 100_000}, ""},
		{"any station when network rule off", func(f fx) { f.fleet.rules.Network = false }, AuthorizeInput{StationID: "s3", Pump: 1, Fuel: common.Fuel95, Amount: 100_000}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFx()
			_, _ = f.svc.RecordOdometer(ctx, "d3", "142880")
			if tc.setup != nil {
				tc.setup(f)
			}
			_, err := f.svc.Authorize(ctx, "d3", tc.in)
			if code(err) != tc.want {
				t.Fatalf("code = %q, want %q", code(err), tc.want)
			}
			if tc.want != "" && len(f.pay.charges) != 0 {
				t.Fatal("a rejected authorization must not charge")
			}
		})
	}
}

func TestAuthorizeCompensatesWhenRecordingFails(t *testing.T) {
	f := newFx()
	_, _ = f.svc.RecordOdometer(ctx, "d3", "142880")
	f.store.st.failInsert = true
	if _, err := f.svc.Authorize(ctx, "d3", authInput(100_000)); err == nil {
		t.Fatal("expected failure")
	}
	if len(f.pay.refunds) != 1 || f.pay.balance != 18_450_000 {
		t.Fatalf("refunds = %v, balance %d", f.pay.refunds, f.pay.balance)
	}
	if st, _ := f.svc.PassStatus(ctx, "d3"); !st.Odometer.Recorded {
		t.Fatal("odometer reading must survive a failed authorization")
	}
}

func TestAuthorizePaymentErrorPropagates(t *testing.T) {
	f := newFx()
	_, _ = f.svc.RecordOdometer(ctx, "d3", "142880")
	f.pay.failNext = httpx.Conflict("insufficient_balance", "x", "x")
	if _, err := f.svc.Authorize(ctx, "d3", authInput(100_000)); code(err) != "insufficient_balance" {
		t.Fatalf("code = %q", code(err))
	}
	if len(f.store.st.txs) != 0 {
		t.Fatal("no transaction may be recorded when the charge fails")
	}
}

// ---------- station flow ----------

func TestScanQR(t *testing.T) {
	f := newFx()
	if _, err := f.svc.Scan(ctx, attendant, ScanInput{Method: SourceQR, Token: "PF-nope"}); code(err) != "pass_expired" {
		t.Fatalf("unknown token: %q", code(err))
	}
	p, _ := f.svc.IssuePass(ctx, "d3")
	if _, err := f.svc.Scan(ctx, attendant, ScanInput{Method: SourceQR, Token: p.Token}); code(err) != "pass_inactive" {
		t.Fatalf("no odometer: %q", code(err))
	}
	_, _ = f.svc.RecordOdometer(ctx, "d3", "142880")
	res, err := f.svc.Scan(ctx, attendant, ScanInput{Method: SourceQR, Token: p.Token})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Active || res.Vehicle.ID != "f3" || res.Driver.ID != "d3" || res.Available != 400_000 || !res.FuelLocked || res.ScanID == "" || res.Token != p.Token {
		t.Fatalf("scan = %+v", res)
	}
	if _, err := f.svc.Scan(ctx, &authn.Claims{Role: authn.RoleAttendant, Station: "s3"}, ScanInput{Method: SourceQR, Token: p.Token}); code(err) != "station_not_in_network" {
		t.Fatalf("outside network: %q", code(err))
	}
	// Expired token (30s + grace).
	f.kv.Now = func() time.Time { return time.Now().Add(36 * time.Second) }
	if _, err := f.svc.Scan(ctx, attendant, ScanInput{Method: SourceQR, Token: p.Token}); code(err) != "pass_expired" {
		t.Fatalf("expired token: %q", code(err))
	}
}

func TestScanReassignedVehicleInvalidatesPass(t *testing.T) {
	f := newFx()
	p, _ := f.svc.IssuePass(ctx, "d3")
	f.fleet.assign["f3"] = "d6"
	if _, err := f.svc.Scan(ctx, attendant, ScanInput{Method: SourceQR, Token: p.Token}); code(err) != "pass_expired" {
		t.Fatalf("code = %q", code(err))
	}
}

func TestScanSuspendedLogsSecurityEvent(t *testing.T) {
	f := newFx()
	p, _ := f.svc.IssuePass(ctx, "d3")
	f.fleet.drivers["d3"].Active = false
	res, err := f.svc.Scan(ctx, attendant, ScanInput{Method: SourceQR, Token: p.Token})
	if err != nil {
		t.Fatal(err)
	}
	if res.Active || res.Block == nil || res.Block.Kind != "driver" {
		t.Fatalf("scan = %+v", res)
	}
	if n := len(f.store.st.security); n != 1 || f.store.st.security[0].Ref != "D-3071" || f.store.st.security[0].Severity != SeverityCritical {
		t.Fatalf("security log = %+v", f.store.st.security)
	}

	f.fleet.vehicles["f6"].Active = false
	if _, err := f.svc.Scan(ctx, attendant, ScanInput{Method: SourceRFID, VehicleID: "f6"}); err != nil {
		t.Fatal(err)
	}
	if ev := f.store.st.security[1]; ev.Ref != "9087-BH" || ev.Title.En != "Fueling attempt on a suspended vehicle" {
		t.Fatalf("vehicle event = %+v", ev)
	}
}

func TestScanValidation(t *testing.T) {
	f := newFx()
	if _, err := f.svc.Scan(ctx, attendant, ScanInput{Method: "nfc"}); code(err) != "invalid_method" {
		t.Fatalf("method: %q", code(err))
	}
	if _, err := f.svc.Scan(ctx, attendant, ScanInput{Method: SourceRFID}); code(err) != "invalid_method" {
		t.Fatalf("rfid without vehicle: %q", code(err))
	}
}

func TestDispenseByLitresViaRFID(t *testing.T) {
	f := newFx()
	res, err := f.svc.Scan(ctx, attendant, ScanInput{Method: SourceRFID, VehicleID: "f6"})
	if err != nil {
		t.Fatal(err)
	}
	// RFID needs no odometer reading. 60 L × 3,250 = 195,000.
	tx, err := f.svc.Dispense(ctx, attendant, DispenseInput{ScanID: res.ScanID, Pump: 8, Litres: 60})
	if err != nil {
		t.Fatal(err)
	}
	if tx.Status != StatusFueling || tx.Amount != 195_000 || tx.Source != SourceRFID || tx.DriverID != "d6" || tx.StartedAt == nil || tx.Fuel != common.FuelDiesel {
		t.Fatalf("tx = %+v", tx)
	}
	if found, _ := f.kv.GetJSON(ctx, "scan:"+res.ScanID, &scanSession{}); found {
		t.Fatal("scan session must be single-use")
	}
	// The same pump is now busy.
	res2, _ := f.svc.Scan(ctx, attendant, ScanInput{Method: SourceRFID, VehicleID: "f6"})
	if _, err := f.svc.Dispense(ctx, attendant, DispenseInput{ScanID: res2.ScanID, Pump: 8, Amount: 50_000}); code(err) != "pump_busy" {
		t.Fatalf("busy pump: %q", code(err))
	}
}

func TestDispenseRejections(t *testing.T) {
	cases := []struct {
		name  string
		setup func(f fx)
		c     *authn.Claims
		in    func(scanID string) DispenseInput
		want  string
	}{
		{"unknown scan", nil, attendant, func(string) DispenseInput { return DispenseInput{ScanID: "nope", Pump: 8, Amount: 1} }, "scan_expired"},
		{"other station's scan", nil, otherStn, func(id string) DispenseInput { return DispenseInput{ScanID: id, Pump: 8, Amount: 1} }, "scan_expired"},
		{"vehicle suspended after scan", func(f fx) { f.fleet.vehicles["f6"].Active = false }, attendant, func(id string) DispenseInput { return DispenseInput{ScanID: id, Pump: 8, Amount: 1} }, "blocked"},
		{"offline pump", nil, attendant, func(id string) DispenseInput { return DispenseInput{ScanID: id, Pump: 7, Amount: 1} }, "pump_unavailable"},
		{"pump without the vehicle's grade", nil, attendant, func(id string) DispenseInput { return DispenseInput{ScanID: id, Pump: 3, Amount: 1} }, "pump_unavailable"},
		{"zero quantity", nil, attendant, func(id string) DispenseInput { return DispenseInput{ScanID: id, Pump: 8} }, "invalid_amount"},
		{"over remaining limit", nil, attendant, func(id string) DispenseInput { return DispenseInput{ScanID: id, Pump: 8, Amount: 360_001} }, "exceeds_available"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFx()
			res, _ := f.svc.Scan(ctx, attendant, ScanInput{Method: SourceRFID, VehicleID: "f6"})
			if tc.setup != nil {
				tc.setup(f)
			}
			if _, err := f.svc.Dispense(ctx, tc.c, tc.in(res.ScanID)); code(err) != tc.want {
				t.Fatalf("code = %q, want %q", code(err), tc.want)
			}
		})
	}
}

func TestLifecycleAndOutbox(t *testing.T) {
	f := newFx()
	_, _ = f.svc.RecordOdometer(ctx, "d3", "142880")
	tx, _ := f.svc.Authorize(ctx, "d3", authInput(150_000))

	if _, err := f.svc.Finish(ctx, attendant, tx.ID); code(err) != "invalid_state" {
		t.Fatalf("finish before start: %q", code(err))
	}
	if _, err := f.svc.Start(ctx, otherStn, tx.ID); code(err) != "not_found" {
		t.Fatalf("other station start: %q", code(err))
	}
	if _, err := f.svc.StartByCode(ctx, attendant, "0000"); code(err) != "code_not_found" {
		t.Fatalf("bad code: %q", code(err))
	}
	started, err := f.svc.StartByCode(ctx, attendant, tx.Code)
	if err != nil || started.Status != StatusFueling || started.StartedAt == nil {
		t.Fatalf("start by code = %+v, %v", started, err)
	}
	if _, err := f.svc.Start(ctx, attendant, tx.ID); code(err) != "invalid_state" {
		t.Fatalf("double start: %q", code(err))
	}
	if len(f.store.st.outbox) != 0 {
		t.Fatal("no event before completion")
	}
	done, err := f.svc.Finish(ctx, attendant, tx.ID)
	if err != nil || done.Status != StatusDone || done.CompletedAt == nil {
		t.Fatalf("finish = %+v, %v", done, err)
	}
	if len(f.store.st.outbox) != 1 || f.store.st.outbox[0].Type != events.TypeTransactionCompleted {
		t.Fatalf("outbox = %+v", f.store.st.outbox)
	}
	var p events.TransactionCompleted
	_ = f.store.st.outbox[0].Decode(&p)
	if p.TransactionID != tx.ID || p.StationID != "s1" || p.Amount != 150_000 || p.OrgID != "o1" {
		t.Fatalf("payload = %+v", p)
	}
}

func TestTransactionsAreScopedByRole(t *testing.T) {
	f := newFx()
	f.store.st.txs = map[string]Transaction{
		"T-1": {ID: "T-1", OrgID: "o1", DriverID: "d3", StationID: "s1", Amount: 3600, PricePerLitre: 3600},
		"T-2": {ID: "T-2", OrgID: "o1", DriverID: "d6", StationID: "s2", Amount: 1, PricePerLitre: 1},
		"T-3": {ID: "T-3", OrgID: "", StationID: "s1", Amount: 1, PricePerLitre: 1},
	}
	count := func(c *authn.Claims) int {
		ts, err := f.svc.Transactions(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		return len(ts)
	}
	if n := count(&authn.Claims{Role: authn.RoleDriver, Actor: "d3"}); n != 1 {
		t.Errorf("driver sees %d", n)
	}
	if n := count(&authn.Claims{Role: authn.RoleFleetManager, Org: "o1"}); n != 2 {
		t.Errorf("fleet manager sees %d", n)
	}
	if n := count(attendant); n != 2 {
		t.Errorf("station sees %d", n)
	}
	ts, _ := f.svc.Transactions(ctx, &authn.Claims{Role: authn.RoleDriver, Actor: "d3"})
	if ts[0].Litres != 1 {
		t.Fatalf("litres not derived: %+v", ts[0])
	}
}

func TestHTTPGuards(t *testing.T) {
	env := authntest.New(t)
	f := newFx()
	c := env.Server(NewAPI(f.svc).Register)
	drv := env.Token(authn.RoleDriver, "o1", "", "d3")
	att := env.Token(authn.RoleAttendant, "", "s1", "a2")
	fm := env.Token(authn.RoleFleetManager, "o1", "", "m1")

	cases := []struct {
		method, path, tok string
		body              any
		want              int
	}{
		{"GET", "/api/v1/fueling/me/pass", drv, nil, 200},
		{"GET", "/api/v1/fueling/me/pass", att, nil, 403},
		{"POST", "/api/v1/fueling/me/odometer", drv, map[string]string{"km": "142880"}, 201},
		{"POST", "/api/v1/fueling/me/pass-token", drv, nil, 201},
		{"POST", "/api/v1/fueling/authorizations", att, authInput(100_000), 403},
		{"POST", "/api/v1/fueling/authorizations", drv, authInput(100_000), 201},
		{"POST", "/api/v1/fueling/scans", drv, ScanInput{Method: SourceRFID, VehicleID: "f6"}, 403},
		{"POST", "/api/v1/fueling/scans", att, ScanInput{Method: SourceRFID, VehicleID: "f6"}, 201},
		{"GET", "/api/v1/fueling/security-events", fm, nil, 200},
		{"GET", "/api/v1/fueling/security-events", drv, nil, 403},
		{"GET", "/api/v1/fueling/transactions", fm, nil, 200},
		{"POST", "/api/v1/fueling/transactions/T-404/start", att, nil, 404},
	}
	for _, tc := range cases {
		if got := c.Do(tc.method, tc.path, tc.tok, tc.body, nil); got != tc.want {
			t.Errorf("%s %s: %d, want %d", tc.method, tc.path, got, tc.want)
		}
	}
	var ts []Transaction
	if s := c.Do("GET", "/api/v1/fueling/transactions", drv, nil, &ts); s != http.StatusOK || len(ts) != 1 {
		t.Fatalf("driver transactions: %d %d", s, len(ts))
	}
}
