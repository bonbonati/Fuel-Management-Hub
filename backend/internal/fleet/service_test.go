package fleet

import (
	"context"
	"testing"

	"github.com/bonbonati/fuel-management-hub/backend/internal/common"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/i18n"
)

var ctx = context.Background()

func newSvc() (*Service, memStore) {
	st := newSeededStore()
	return NewService(st), st
}

func codeOf(err error) string {
	if e := httpx.AsError(err); e != nil {
		return e.Code
	}
	if err != nil {
		return "untyped: " + err.Error()
	}
	return ""
}

func driverOf(st memStore, vehicleID string) string {
	if d := st.st.vehicles[vehicleID].DriverID; d != nil {
		return *d
	}
	return ""
}

func TestSaveDriverAssignsVehicleExclusively(t *testing.T) {
	svc, st := newSvc()
	// Move d3 from f3 onto f1 (currently d1's).
	d, err := svc.SaveDriver(ctx, "o1", "d3", DriverInput{
		Name: i18n.T("عبدالرحمن", "Abdurahman"), Code: "D-3071", Phone: "+249 90 552 4471", Active: true, VehicleID: ptr("f1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Phone != "+249 90 552 4471" {
		t.Fatalf("phone = %q", d.Phone)
	}
	if driverOf(st, "f1") != "d3" || driverOf(st, "f3") != "" {
		t.Fatalf("f1=%q f3=%q, want d3 on f1 and f3 unassigned", driverOf(st, "f1"), driverOf(st, "f3"))
	}
}

func TestCreateDriverWithVehicle(t *testing.T) {
	svc, st := newSvc()
	d, err := svc.SaveDriver(ctx, "o1", "", DriverInput{
		Name: i18n.T("جديد", "New"), Code: "D-4000", Phone: "249911112222", Active: true, VehicleID: ptr("f6"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.ID == "" || d.OrgID != "o1" || d.Phone != "+249 91 111 2222" {
		t.Fatalf("driver = %+v", d)
	}
	if driverOf(st, "f6") != d.ID {
		t.Fatal("new driver not assigned to f6")
	}
}

func TestSaveDriverValidation(t *testing.T) {
	svc, st := newSvc()
	base := DriverInput{Name: i18n.T("أ", "A"), Code: "D-5000", Phone: "+249 90 000 0000", Active: true}
	cases := []struct {
		name string
		id   string
		mod  func(*DriverInput)
		want string
	}{
		{"bad phone", "", func(in *DriverInput) { in.Phone = "+966 1" }, "invalid_phone"},
		{"missing english name", "", func(in *DriverInput) { in.Name.En = " " }, "invalid_field"},
		{"missing code", "", func(in *DriverInput) { in.Code = "" }, "invalid_field"},
		{"duplicate code", "", func(in *DriverInput) { in.Code = "D-3012" }, "duplicate_code"},
		{"vehicle of another org", "", func(in *DriverInput) { in.VehicleID = ptr("fx") }, "invalid_field"},
		{"driver of another org", "dx", func(*DriverInput) {}, "not_found"},
	}
	before := len(st.st.drivers)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			tc.mod(&in)
			if _, err := svc.SaveDriver(ctx, "o1", tc.id, in); codeOf(err) != tc.want {
				t.Fatalf("code = %q, want %q", codeOf(err), tc.want)
			}
		})
	}
	if len(st.st.drivers) != before {
		t.Fatal("failed saves must not leave partial writes")
	}
}

func TestSaveVehicle(t *testing.T) {
	svc, st := newSvc()
	v, err := svc.SaveVehicle(ctx, "o1", "", VehicleInput{
		Plate: common.Plate{Digits: "1234", Latin: "krt"}, Name: i18n.T("نيسان", "Nissan"), Fuel: common.Fuel91,
		TierID: ptr("t2"), DriverID: ptr("d1"), TagBattery: 140, Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if v.Plate.Latin != "KRT" || v.Plate.Arabic != "خ ر ط" || v.TagBattery != 100 || v.Extra != 0 {
		t.Fatalf("vehicle = %+v", v)
	}
	if v.Limit != 1_200_000 {
		t.Fatalf("limit = %d, want tier budget", v.Limit)
	}
	if driverOf(st, v.ID) != "d1" || driverOf(st, "f1") != "" {
		t.Fatal("d1 must move to the new vehicle and leave f1")
	}

	// Update: drop the driver and set a custom limit.
	v2, err := svc.SaveVehicle(ctx, "o1", v.ID, VehicleInput{
		Plate: v.Plate, Name: v.Name, Fuel: v.Fuel, CustomLimit: ptr(int64(500_000)), TagBattery: -5, Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if v2.DriverID != nil || v2.TierID != nil || v2.Limit != 500_000 || v2.TagBattery != 0 {
		t.Fatalf("updated vehicle = %+v", v2)
	}
}

func TestSaveVehicleValidation(t *testing.T) {
	svc, _ := newSvc()
	ok := VehicleInput{Plate: common.Plate{Digits: "1", Latin: "K"}, Name: i18n.T("أ", "A"), Fuel: common.Fuel91}
	cases := map[string]func(*VehicleInput){
		"bad plate":         func(in *VehicleInput) { in.Plate.Digits = "12ab" },
		"bad fuel":          func(in *VehicleInput) { in.Fuel = "98" },
		"missing name":      func(in *VehicleInput) { in.Name = i18n.Text{} },
		"negative custom":   func(in *VehicleInput) { in.CustomLimit = ptr(int64(-1)) },
		"foreign tier":      func(in *VehicleInput) { in.TierID = ptr("tx") },
		"foreign driver":    func(in *VehicleInput) { in.DriverID = ptr("dx") },
		"unknown driver id": func(in *VehicleInput) { in.DriverID = ptr("nope") },
	}
	for name, mod := range cases {
		t.Run(name, func(t *testing.T) {
			in := ok
			mod(&in)
			if _, err := svc.SaveVehicle(ctx, "o1", "", in); codeOf(err) != "invalid_field" {
				t.Fatalf("code = %q", codeOf(err))
			}
		})
	}
	if _, err := svc.SaveVehicle(ctx, "o1", "fx", ok); codeOf(err) != "not_found" {
		t.Fatalf("other org's vehicle: code = %q", codeOf(err))
	}
}

func TestSaveTierAssignsVehicles(t *testing.T) {
	svc, st := newSvc()
	tier, err := svc.SaveTier(ctx, "o1", "", TierInput{Name: i18n.T("جديدة", "New"), Budget: 900_000, DailyLitres: 40, Window: "08:00–18:00", VehicleIDs: []string{"f1", "f3"}})
	if err != nil {
		t.Fatal(err)
	}
	if tier.Icon != "layers" || tier.Color == "" {
		t.Fatalf("new tier defaults = %+v", tier)
	}
	if *st.st.vehicles["f1"].TierID != tier.ID || *st.st.vehicles["f3"].TierID != tier.ID {
		t.Fatal("listed vehicles not moved into tier")
	}
	// Emptying the tier releases its vehicles.
	if _, err := svc.SaveTier(ctx, "o1", tier.ID, TierInput{Name: tier.Name, Budget: 900_000, DailyLitres: 40, Window: "x"}); err != nil {
		t.Fatal(err)
	}
	if st.st.vehicles["f1"].TierID != nil || st.st.vehicles["f3"].TierID != nil {
		t.Fatal("unlisted vehicles must leave the tier")
	}
	if *st.st.vehicles["f6"].TierID != "t1" {
		t.Fatal("vehicles of other tiers must be untouched")
	}
}

func TestSaveTierRejectsForeignVehicleAtomically(t *testing.T) {
	svc, st := newSvc()
	n := len(st.st.tiers)
	_, err := svc.SaveTier(ctx, "o1", "", TierInput{Name: i18n.T("أ", "A"), Budget: 20_000, DailyLitres: 1, Window: "x", VehicleIDs: []string{"f1", "fx"}})
	if codeOf(err) != "invalid_field" {
		t.Fatalf("code = %q", codeOf(err))
	}
	if len(st.st.tiers) != n || *st.st.vehicles["f1"].TierID != "t1" {
		t.Fatal("rejected tier save left partial writes")
	}
	for _, bad := range []TierInput{
		{Name: i18n.T("أ", ""), Budget: 20_000, DailyLitres: 1, Window: "x"},
		{Name: i18n.T("أ", "A"), Budget: 9_999, DailyLitres: 1, Window: "x"},
		{Name: i18n.T("أ", "A"), Budget: 20_000, DailyLitres: 0, Window: "x"},
		{Name: i18n.T("أ", "A"), Budget: 20_000, DailyLitres: 1, Window: " "},
	} {
		if _, err := svc.SaveTier(ctx, "o1", "", bad); codeOf(err) != "invalid_field" {
			t.Errorf("tier %+v: code = %q", bad, codeOf(err))
		}
	}
}

func TestVehicleToggles(t *testing.T) {
	svc, st := newSvc()
	if v, err := svc.SetVehicleActive(ctx, "o1", "f3", false); err != nil || v.Active {
		t.Fatalf("suspend: %+v %v", v, err)
	}
	if v, err := svc.SetVehicleTier(ctx, "o1", "f3", "t1"); err != nil || v.Limit != 1_500_000+150_000 {
		t.Fatalf("tier move: limit %d, %v", v.Limit, err)
	}
	if v, _ := svc.SetVehicleTier(ctx, "o1", "f3", ""); v.TierID != nil || v.Limit != 150_000 {
		t.Fatalf("tier removal: %+v", v)
	}
	if _, err := svc.SetVehicleTier(ctx, "o1", "f3", "tx"); codeOf(err) != "invalid_field" {
		t.Fatal("foreign tier must be rejected")
	}
	if v, err := svc.IncreaseLimit(ctx, "o1", "f3", 300_000); err != nil || st.st.vehicles["f3"].Extra != 450_000 || v.Extra != 450_000 {
		t.Fatalf("increase: %+v %v", v, err)
	}
	if _, err := svc.IncreaseLimit(ctx, "o1", "f3", 0); codeOf(err) != "invalid_amount" {
		t.Fatal("zero increase must be rejected")
	}
	if _, err := svc.SetVehicleActive(ctx, "o1", "fx", false); codeOf(err) != "not_found" {
		t.Fatal("other org's vehicle must be invisible")
	}
	if d, err := svc.SetDriverActive(ctx, "o1", "d3", false); err != nil || d.Active {
		t.Fatalf("driver suspend: %+v %v", d, err)
	}
}

func TestVehiclesListComputesLimits(t *testing.T) {
	svc, _ := newSvc()
	vs, err := svc.Vehicles(ctx, "o1")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"f1": 1_500_000, "f3": 1_350_000, "f6": 1_800_000}
	if len(vs) != len(want) {
		t.Fatalf("got %d vehicles (tenant leak?)", len(vs))
	}
	for _, v := range vs {
		if v.Limit != want[v.ID] {
			t.Errorf("%s limit = %d, want %d", v.ID, v.Limit, want[v.ID])
		}
	}
}

func TestPatchRules(t *testing.T) {
	svc, _ := newSvc()
	r, err := svc.PatchRules(ctx, "o1", RulesPatch{Odometer: ptr(false), MaxFillAmount: ptr(int64(500_000))})
	if err != nil {
		t.Fatal(err)
	}
	if r.Odometer || r.MaxFillAmount != 500_000 || !r.Network || len(r.NetworkStationIDs) != 2 {
		t.Fatalf("rules = %+v", r)
	}
	if _, err := svc.PatchRules(ctx, "o1", RulesPatch{MaxFillAmount: ptr(int64(5_000))}); codeOf(err) != "invalid_amount" {
		t.Fatalf("code = %q", codeOf(err))
	}
}

func TestUpdateOrg(t *testing.T) {
	svc, _ := newSvc()
	o, err := svc.UpdateOrg(ctx, "o1", OrgPatch{Phone: ptr(" +249 1 "), Name: &i18n.Text{Ar: "ن", En: "N"}})
	if err != nil || o.Phone != "+249 1" || o.Name.En != "N" {
		t.Fatalf("org = %+v, %v", o, err)
	}
	if _, err := svc.UpdateOrg(ctx, "o1", OrgPatch{Phone: ptr(" ")}); codeOf(err) != "invalid_field" {
		t.Fatal("blank phone must be rejected")
	}
}

func TestCreateRequestFlow(t *testing.T) {
	svc, st := newSvc()
	req, err := svc.CreateRequest(ctx, "o1", "d3", RequestInput{Type: RequestLimit, Amount: 300_000, Reason: "trip", Note: "  Wad Madani  "})
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != StatusPending || req.VehicleID == nil || *req.VehicleID != "f3" || req.Note.En != "Wad Madani" {
		t.Fatalf("request = %+v", req)
	}
	if _, err := svc.CreateRequest(ctx, "o1", "d3", RequestInput{Type: RequestLimit, Amount: 100_000, Reason: "trip"}); codeOf(err) != "duplicate_request" {
		t.Fatalf("duplicate: code = %q", codeOf(err))
	}
	// A suspended driver can only ask for reactivation.
	d := st.st.drivers["d1"]
	d.Active = false
	st.st.drivers["d1"] = d
	if _, err := svc.CreateRequest(ctx, "o1", "d1", RequestInput{Type: RequestLimit, Amount: 100_000, Reason: "trip"}); codeOf(err) != "limit_unavailable" {
		t.Fatalf("suspended limit: code = %q", codeOf(err))
	}
	if _, err := svc.CreateRequest(ctx, "o1", "d1", RequestInput{Type: RequestActivate, Reason: "review"}); err != nil {
		t.Fatal(err)
	}
	// Drivers of another org are invisible.
	if _, err := svc.CreateRequest(ctx, "o1", "dx", RequestInput{Type: RequestLimit, Amount: 100_000, Reason: "trip"}); codeOf(err) != "not_found" {
		t.Fatalf("cross-org: code = %q", codeOf(err))
	}
}

func TestDecideLimit(t *testing.T) {
	svc, st := newSvc()
	req, _ := svc.CreateRequest(ctx, "o1", "d3", RequestInput{Type: RequestLimit, Amount: 300_000, Reason: "trip"})

	if _, err := svc.Decide(ctx, "o1", req.ID, DecisionInput{Approve: true, Amount: ptr(int64(5_000))}); codeOf(err) != "invalid_amount" {
		t.Fatalf("small approval: code = %q", codeOf(err))
	}
	if st.st.vehicles["f3"].Extra != 150_000 {
		t.Fatal("rejected approval must not change the vehicle")
	}
	got, err := svc.Decide(ctx, "o1", req.ID, DecisionInput{Approve: true, Amount: ptr(int64(250_000)), Note: " ok "})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusApproved || got.Approved != 250_000 || got.DecidedAt == nil || got.Reply.En != "ok" {
		t.Fatalf("decision = %+v", got)
	}
	if st.st.vehicles["f3"].Extra != 400_000 {
		t.Fatalf("extra = %d, want 150k+250k", st.st.vehicles["f3"].Extra)
	}
	if _, err := svc.Decide(ctx, "o1", req.ID, DecisionInput{Approve: true}); codeOf(err) != "not_pending" {
		t.Fatal("deciding twice must fail")
	}
	if _, err := svc.Decide(ctx, "o2", "R-1043", DecisionInput{}); codeOf(err) != "not_found" {
		t.Fatal("other org must not decide")
	}
}

func TestDecideLimitWithoutVehicle(t *testing.T) {
	svc, _ := newSvc()
	// The seeded R-1043 has no vehicle attached.
	if _, err := svc.Decide(ctx, "o1", "R-1043", DecisionInput{Approve: true}); codeOf(err) != "no_vehicle" {
		t.Fatalf("code = %q", codeOf(err))
	}
	got, err := svc.Decide(ctx, "o1", "R-1043", DecisionInput{Approve: false})
	if err != nil || got.Status != StatusRejected {
		t.Fatalf("reject = %+v, %v", got, err)
	}
}

func TestDecideActivationReactivatesDriverAndVehicle(t *testing.T) {
	svc, st := newSvc()
	_, _ = svc.SetDriverActive(ctx, "o1", "d3", false)
	_, _ = svc.SetVehicleActive(ctx, "o1", "f3", false)
	req, err := svc.CreateRequest(ctx, "o1", "d3", RequestInput{Type: RequestActivate, Reason: "review"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Decide(ctx, "o1", req.ID, DecisionInput{Approve: true}); err != nil {
		t.Fatal(err)
	}
	if !st.st.drivers["d3"].Active || !st.st.vehicles["f3"].Active {
		t.Fatal("approval must reactivate driver and vehicle")
	}
}

func TestRequestsFilter(t *testing.T) {
	svc, _ := newSvc()
	_, _ = svc.CreateRequest(ctx, "o1", "d3", RequestInput{Type: RequestLimit, Amount: 50_000, Reason: "trip"})
	all, _ := svc.Requests(ctx, "o1", "all", "")
	pend, _ := svc.Requests(ctx, "o1", "pending", "")
	mine, _ := svc.Requests(ctx, "o1", "", "d3")
	if len(all) != 2 || len(pend) != 2 || len(mine) != 1 || mine[0].DriverID != "d3" {
		t.Fatalf("all=%d pending=%d mine=%d", len(all), len(pend), len(mine))
	}
	if all[0].DriverID != "d3" {
		t.Fatal("requests must be newest first")
	}
}

func TestContexts(t *testing.T) {
	svc, st := newSvc()
	c, err := svc.DriverContext(ctx, "d3")
	if err != nil {
		t.Fatal(err)
	}
	if c.OrgID != "o1" || c.Vehicle == nil || c.Vehicle.ID != "f3" || c.Vehicle.Limit != 1_350_000 || c.Tier.ID != "t2" || c.Block != nil || !c.Rules.WrongFuel {
		t.Fatalf("driver context = %+v", c)
	}

	// Driver without a vehicle is blocked with "noveh".
	_, _ = svc.SaveDriver(ctx, "o1", "d4", DriverInput{Name: i18n.T("سعد", "Saad"), Code: "D-3088", Phone: "+249 99 407 0952", Active: true})
	if c, _ := svc.DriverContext(ctx, "d4"); c.Block == nil || c.Block.Kind != BlockNoVehicle {
		t.Fatalf("d4 block = %+v", c.Block)
	}

	vc, err := svc.VehicleContext(ctx, "f6")
	if err != nil || vc.Driver != nil || vc.Block != nil || vc.Vehicle.Limit != 1_800_000 {
		t.Fatalf("vehicle context = %+v, %v", vc, err)
	}
	v := st.st.vehicles["f6"]
	v.Active = false
	st.st.vehicles["f6"] = v
	if vc, _ := svc.VehicleContext(ctx, "f6"); vc.Block == nil || vc.Block.Kind != BlockVehicle {
		t.Fatal("suspended vehicle must be blocked")
	}
	if _, err := svc.VehicleContext(ctx, "nope"); codeOf(err) != "not_found" {
		t.Fatal("unknown vehicle must be not found")
	}
}
