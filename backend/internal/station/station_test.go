package station

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/bonbonati/fuel-management-hub/backend/internal/common"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn/authntest"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/i18n"
)

// memRepo is an in-memory Repo for tests.
type memRepo struct {
	stations []Station
	pumps    []Pump
	staff    []Staff
}

func (m *memRepo) ListStations(context.Context) ([]Station, error) {
	out := slices.Clone(m.stations)
	for i := range out {
		for _, p := range m.pumps {
			if p.StationID == out[i].ID {
				out[i].PumpCount++
			}
		}
	}
	return out, nil
}
func (m *memRepo) GetStation(ctx context.Context, id string) (Station, error) {
	all, _ := m.ListStations(ctx)
	for _, s := range all {
		if s.ID == id {
			return s, nil
		}
	}
	return Station{}, ErrNotFound
}
func (m *memRepo) UpdateStation(_ context.Context, s Station) error {
	for i := range m.stations {
		if m.stations[i].ID == s.ID {
			m.stations[i] = s
		}
	}
	return nil
}
func (m *memRepo) ListPumps(_ context.Context, sid string) ([]Pump, error) {
	var out []Pump
	for _, p := range m.pumps {
		if p.StationID == sid {
			out = append(out, p)
		}
	}
	return out, nil
}
func (m *memRepo) GetPump(_ context.Context, sid string, n int) (Pump, error) {
	for _, p := range m.pumps {
		if p.StationID == sid && p.Number == n {
			return p, nil
		}
	}
	return Pump{}, ErrNotFound
}
func (m *memRepo) UpdatePump(_ context.Context, p Pump) error {
	for i := range m.pumps {
		if m.pumps[i].StationID == p.StationID && m.pumps[i].Number == p.Number {
			m.pumps[i] = p
		}
	}
	return nil
}
func (m *memRepo) ListStaff(_ context.Context, sid string) ([]Staff, error) {
	var out []Staff
	for _, s := range m.staff {
		if s.StationID == sid {
			out = append(out, s)
		}
	}
	return out, nil
}
func (m *memRepo) GetStaff(_ context.Context, id string) (Staff, error) {
	for _, s := range m.staff {
		if s.ID == id {
			return s, nil
		}
	}
	return Staff{}, ErrNotFound
}
func (m *memRepo) InsertStaff(_ context.Context, s Staff) (string, error) {
	s.ID = fmt.Sprintf("a%d", 100+len(m.staff))
	m.staff = append(m.staff, s)
	return s.ID, nil
}
func (m *memRepo) UpdateStaff(_ context.Context, s Staff) error {
	for i := range m.staff {
		if m.staff[i].ID == s.ID {
			m.staff[i] = s
		}
	}
	return nil
}
func (m *memRepo) Prices(context.Context) (map[common.Fuel]int64, error) {
	return common.DefaultPrices, nil
}

func seeded() *memRepo {
	return &memRepo{
		stations: []Station{
			{ID: "s1", Name: i18n.T("محطة النيل الأزرق", "Blue Nile Station"), Area: i18n.T("شارع أفريقيا", "Africa Rd"), Phone: "+249 18 377 0001"},
			{ID: "s2", Name: i18n.T("وقود المقرن", "Al Mogran Fuel"), Area: i18n.T("شارع النيل", "Nile St"), Phone: "+249 18 377 0002"},
		},
		pumps: []Pump{
			{StationID: "s1", Number: 1, Grades: []common.Fuel{common.Fuel91, common.Fuel95}, FlowRate: 40, Online: true},
			{StationID: "s1", Number: 7, Grades: []common.Fuel{common.FuelDiesel}, FlowRate: 120, Online: false},
			{StationID: "s2", Number: 1, Grades: common.Fuels, FlowRate: 40, Online: true},
		},
		staff: []Staff{
			{ID: "a1", StationID: "s1", Name: i18n.T("مجدي", "Magdi"), Code: "ATT-01", Role: RoleSupervisor, Shift: "A", Active: true},
			{ID: "a2", StationID: "s1", Name: i18n.T("إبراهيم", "Ibrahim"), Code: "ATT-09", Role: RoleAttendant, Shift: "A", Active: true},
			{ID: "b1", StationID: "s2", Name: i18n.T("آخر", "Other"), Code: "B-1", Role: RoleAttendant, Shift: "A", Active: true},
		},
	}
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

func TestUpdatePumpValidation(t *testing.T) {
	svc := NewService(seeded())
	p, err := svc.UpdatePump(ctx, "s1", 1, PumpInput{Grades: []common.Fuel{common.FuelDiesel, common.Fuel91, common.Fuel91}, FlowRate: 60, Online: true})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Grades, []common.Fuel{common.Fuel91, common.FuelDiesel}) || p.FlowRate != 60 {
		t.Fatalf("grades must be de-duplicated in canonical order: %+v", p)
	}
	cases := []struct {
		name string
		in   PumpInput
		want string
	}{
		{"no grades", PumpInput{FlowRate: 40}, "no_grades"},
		{"unknown grade", PumpInput{Grades: []common.Fuel{"98"}, FlowRate: 40}, "invalid_field"},
		{"flow too low", PumpInput{Grades: common.Fuels, FlowRate: 9}, "invalid_field"},
		{"flow too high", PumpInput{Grades: common.Fuels, FlowRate: 201}, "invalid_field"},
	}
	for _, tc := range cases {
		if _, err := svc.UpdatePump(ctx, "s1", 1, tc.in); code(err) != tc.want {
			t.Errorf("%s: code %q, want %q", tc.name, code(err), tc.want)
		}
	}
	if _, err := svc.UpdatePump(ctx, "s1", 99, PumpInput{Grades: common.Fuels, FlowRate: 40}); code(err) != "not_found" {
		t.Error("unknown pump must be not found")
	}
}

func TestPumpOnlineAndQuote(t *testing.T) {
	svc := NewService(seeded())
	if p, err := svc.SetPumpOnline(ctx, "s1", 7, true); err != nil || !p.Online {
		t.Fatalf("online: %+v %v", p, err)
	}
	q, err := svc.Quote(ctx, "s1", 7)
	if err != nil || !q.Pump.Online || q.Prices[common.FuelDiesel] != 3250 {
		t.Fatalf("quote = %+v %v", q, err)
	}
}

func TestSaveStaff(t *testing.T) {
	repo := seeded()
	svc := NewService(repo)
	s, err := svc.SaveStaff(ctx, "s1", "", StaffInput{Name: i18n.T("جديد", "New"), Code: "ATT-13", Role: RoleAttendant, Shift: "C", Device: " Pixel ", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if s.ID == "" || s.StationID != "s1" || s.Device != "Pixel" {
		t.Fatalf("staff = %+v", s)
	}
	cases := []struct {
		name string
		id   string
		in   StaffInput
		want string
	}{
		{"duplicate code (case-insensitive)", "", StaffInput{Name: i18n.T("أ", "A"), Code: "att-09", Role: RoleAttendant, Shift: "A"}, "duplicate_code"},
		{"bad role", "", StaffInput{Name: i18n.T("أ", "A"), Code: "X", Role: "boss", Shift: "A"}, "invalid_field"},
		{"bad shift", "", StaffInput{Name: i18n.T("أ", "A"), Code: "X", Role: RoleAttendant, Shift: "D"}, "invalid_field"},
		{"missing name", "", StaffInput{Code: "X", Role: RoleAttendant, Shift: "A"}, "invalid_field"},
		{"other station's staff", "b1", StaffInput{Name: i18n.T("أ", "A"), Code: "X", Role: RoleAttendant, Shift: "A"}, "not_found"},
	}
	for _, tc := range cases {
		if _, err := svc.SaveStaff(ctx, "s1", tc.id, tc.in); code(err) != tc.want {
			t.Errorf("%s: code %q, want %q", tc.name, code(err), tc.want)
		}
	}
	// Keeping your own code on update is not a duplicate.
	if _, err := svc.SaveStaff(ctx, "s1", "a2", StaffInput{Name: i18n.T("إبراهيم", "Ibrahim"), Code: "ATT-09", Role: RoleAttendant, Shift: "B", Active: true}); err != nil {
		t.Fatal(err)
	}
	if st, _ := svc.SetStaffActive(ctx, "s1", "a2", false); st.Active {
		t.Fatal("staff not suspended")
	}
}

func TestUpdateStationMergesLanguages(t *testing.T) {
	svc := NewService(seeded())
	s, err := svc.UpdateStation(ctx, "s1", StationPatch{Name: &i18n.Text{En: "Blue Nile Fuel"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Name.Ar != "محطة النيل الأزرق" || s.Name.En != "Blue Nile Fuel" {
		t.Fatalf("name = %+v", s.Name)
	}
	if _, err := svc.UpdateStation(ctx, "s1", StationPatch{Phone: new(string)}); code(err) != "empty_field" {
		t.Fatal("blank phone must be rejected")
	}
	if _, err := svc.UpdateStation(ctx, "s1", StationPatch{Area: &i18n.Text{}}); code(err) != "empty_field" {
		t.Fatal("blank area must be rejected")
	}
}

func TestHTTPGuardsAndOwnership(t *testing.T) {
	env := authntest.New(t)
	c := env.Server(NewAPI(NewService(seeded())).Register)
	admin := env.Token(authn.RoleStationAdmin, "", "s1", "a1")
	att := env.Token(authn.RoleAttendant, "", "s1", "a2")
	drv := env.Token(authn.RoleDriver, "o1", "", "d3")

	cases := []struct {
		method, path, tok string
		body              any
		want              int
	}{
		{"GET", "/api/v1/stations", "", nil, 401},
		{"GET", "/api/v1/stations", drv, nil, 200},
		{"GET", "/api/v1/stations/fuel-prices", drv, nil, 200},
		{"GET", "/api/v1/stations/s1/pumps", drv, nil, 200},
		{"GET", "/api/v1/stations/nope", drv, nil, 404},
		{"PUT", "/api/v1/stations/s1/pumps/1", att, PumpInput{Grades: common.Fuels, FlowRate: 40}, 403},
		{"PUT", "/api/v1/stations/s2/pumps/1", admin, PumpInput{Grades: common.Fuels, FlowRate: 40}, 403}, // other station
		{"PUT", "/api/v1/stations/s1/pumps/1", admin, PumpInput{Grades: common.Fuels, FlowRate: 40}, 200},
		{"PUT", "/api/v1/stations/s1/pumps/x", admin, PumpInput{Grades: common.Fuels, FlowRate: 40}, 404},
		{"GET", "/api/v1/stations/s1/staff", admin, nil, 200},
		{"GET", "/api/v1/stations/s1/staff", att, nil, 403},
		{"GET", "/api/v1/stations/staff/me", att, nil, 200},
		{"GET", "/api/v1/stations/staff/me", drv, nil, 403},
		{"GET", "/internal/v1/stations/s1/pumps/1/quote", drv, nil, 200},
	}
	for _, tc := range cases {
		if got := c.Do(tc.method, tc.path, tc.tok, tc.body, nil); got != tc.want {
			t.Errorf("%s %s: %d, want %d", tc.method, tc.path, got, tc.want)
		}
	}

	var me StaffProfile
	c.Do("GET", "/api/v1/stations/staff/me", att, nil, &me)
	if me.Staff.ID != "a2" || me.Station.ID != "s1" || me.Station.PumpCount != 2 {
		t.Fatalf("me = %+v", me)
	}
	var created Staff
	if s := c.Do("POST", "/api/v1/stations/s1/staff", admin, StaffInput{Name: i18n.T("أ", "A"), Code: "N-1", Role: RoleAttendant, Shift: "A"}, &created); s != http.StatusCreated || created.StationID != "s1" {
		t.Fatalf("create staff: %d %+v", s, created)
	}
}
