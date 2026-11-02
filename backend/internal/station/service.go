// Package station is the station service: partner fuel stations, their pumps
// (grades, flow rate, online state), station staff and the regulated fuel
// prices.
//
// Pump "busy" state is not stored here; it is derived from open transactions
// owned by the fueling service.
package station

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/bonbonati/fuel-management-hub/backend/internal/common"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/i18n"
)

// Station is a partner fuel station.
type Station struct {
	ID         string    `json:"id"`
	Name       i18n.Text `json:"name"`
	Area       i18n.Text `json:"area"`       // street / district
	DistanceKm float64   `json:"distanceKm"` // demo stand-in for GPS distance to the driver
	Wait       i18n.Text `json:"wait"`       // demo queue estimate
	Phone      string    `json:"phone"`
	BankMask   string    `json:"bankMask"`  // last digits of the payout account
	PumpCount  int       `json:"pumpCount"` // derived from pumps
}

// Pump is one dispenser at a station.
type Pump struct {
	StationID string        `json:"stationId"`
	Number    int           `json:"number"`
	Grades    []common.Fuel `json:"grades"`
	FlowRate  int           `json:"flowRate"` // litres per minute
	Online    bool          `json:"online"`
}

// StaffRole is a station staff role.
type StaffRole string

const (
	RoleSupervisor StaffRole = "sup"
	RoleAttendant  StaffRole = "att"
)

// Staff is a station employee.
type Staff struct {
	ID        string    `json:"id"`
	StationID string    `json:"stationId"`
	Name      i18n.Text `json:"name"`
	Code      string    `json:"code"`  // e.g. "ATT-09"; unique per station
	Role      StaffRole `json:"role"`  // sup | att
	Shift     string    `json:"shift"` // A 06–14, B 14–22, C 22–06
	Device    string    `json:"device"`
	Active    bool      `json:"active"`
}

// ErrNotFound is returned by Repo getters for missing rows.
var ErrNotFound = errors.New("station: not found")

// Repo is the persistence port of the station service.
type Repo interface {
	ListStations(ctx context.Context) ([]Station, error)
	GetStation(ctx context.Context, id string) (Station, error)
	UpdateStation(ctx context.Context, s Station) error
	ListPumps(ctx context.Context, stationID string) ([]Pump, error)
	GetPump(ctx context.Context, stationID string, n int) (Pump, error)
	UpdatePump(ctx context.Context, p Pump) error
	ListStaff(ctx context.Context, stationID string) ([]Staff, error)
	GetStaff(ctx context.Context, id string) (Staff, error)
	InsertStaff(ctx context.Context, s Staff) (string, error)
	UpdateStaff(ctx context.Context, s Staff) error
	Prices(ctx context.Context) (map[common.Fuel]int64, error)
}

// Service implements the station use cases.
type Service struct{ repo Repo }

// NewService returns a Service over repo.
func NewService(repo Repo) *Service { return &Service{repo: repo} }

var (
	errNotFound  = httpx.NotFound("not_found", "العنصر غير موجود.", "Not found.")
	errNoGrades  = httpx.BadRequest("no_grades", "اختر نوع وقود واحداً على الأقل.", "Choose at least one fuel grade.")
	errFlow      = httpx.BadRequest("invalid_field", "يرجى مراجعة حقل «معدل التدفق»", `Please check the "Flow rate" field`)
	errDupCode   = httpx.Conflict("duplicate_code", "الرقم الوظيفي مستخدم لموظف آخر.", "This staff ID is already used.")
	errEmptyText = httpx.BadRequest("empty_field", "لا يمكن ترك هذا الحقل فارغاً", "This field cannot be empty")
)

func fieldErr(ar, en string) error {
	return httpx.BadRequest("invalid_field", "يرجى مراجعة حقل «"+ar+"»", `Please check the "`+en+`" field`)
}

func mapNotFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return errNotFound
	}
	return err
}

// Stations lists all stations.
func (s *Service) Stations(ctx context.Context) ([]Station, error) { return s.repo.ListStations(ctx) }

// Station returns one station.
func (s *Service) Station(ctx context.Context, id string) (Station, error) {
	st, err := s.repo.GetStation(ctx, id)
	return st, mapNotFound(err)
}

// StationPatch updates editable station details; provided fields must not be blank.
type StationPatch struct {
	Name  *i18n.Text `json:"name"`
	Area  *i18n.Text `json:"area"`
	Phone *string    `json:"phone"`
}

// UpdateStation applies p. Partial bilingual updates (only "ar" or only
// "en") keep the other language unchanged, matching the per-field settings form.
func (s *Service) UpdateStation(ctx context.Context, id string, p StationPatch) (Station, error) {
	st, err := s.repo.GetStation(ctx, id)
	if err != nil {
		return Station{}, mapNotFound(err)
	}
	merge := func(dst *i18n.Text, src *i18n.Text) error {
		if src == nil {
			return nil
		}
		n := *dst
		if v := strings.TrimSpace(src.Ar); v != "" {
			n.Ar = v
		}
		if v := strings.TrimSpace(src.En); v != "" {
			n.En = v
		}
		if src.Trimmed().IsZero() {
			return errEmptyText
		}
		*dst = n
		return nil
	}
	if err := merge(&st.Name, p.Name); err != nil {
		return Station{}, err
	}
	if err := merge(&st.Area, p.Area); err != nil {
		return Station{}, err
	}
	if p.Phone != nil {
		ph := strings.TrimSpace(*p.Phone)
		if ph == "" {
			return Station{}, errEmptyText
		}
		st.Phone = ph
	}
	return st, s.repo.UpdateStation(ctx, st)
}

// Pumps lists a station's pumps in number order.
func (s *Service) Pumps(ctx context.Context, stationID string) ([]Pump, error) {
	if _, err := s.repo.GetStation(ctx, stationID); err != nil {
		return nil, mapNotFound(err)
	}
	return s.repo.ListPumps(ctx, stationID)
}

// PumpInput replaces a pump's configuration.
type PumpInput struct {
	Grades   []common.Fuel `json:"grades"`
	FlowRate int           `json:"flowRate"`
	Online   bool          `json:"online"`
}

// UpdatePump validates and saves a pump: at least one known grade
// (duplicates removed, display order kept) and a flow rate of 10–200 L/min.
func (s *Service) UpdatePump(ctx context.Context, stationID string, n int, in PumpInput) (Pump, error) {
	p, err := s.repo.GetPump(ctx, stationID, n)
	if err != nil {
		return Pump{}, mapNotFound(err)
	}
	grades := make([]common.Fuel, 0, len(common.Fuels))
	for _, f := range common.Fuels { // canonical order, de-duplicated
		if slices.Contains(in.Grades, f) {
			grades = append(grades, f)
		}
	}
	for _, g := range in.Grades {
		if !g.Valid() {
			return Pump{}, fieldErr("أنواع الوقود", "Fuel grades")
		}
	}
	if len(grades) == 0 {
		return Pump{}, errNoGrades
	}
	if in.FlowRate < 10 || in.FlowRate > 200 {
		return Pump{}, errFlow
	}
	p.Grades, p.FlowRate, p.Online = grades, in.FlowRate, in.Online
	return p, s.repo.UpdatePump(ctx, p)
}

// SetPumpOnline takes a pump offline or back online.
func (s *Service) SetPumpOnline(ctx context.Context, stationID string, n int, online bool) (Pump, error) {
	p, err := s.repo.GetPump(ctx, stationID, n)
	if err != nil {
		return Pump{}, mapNotFound(err)
	}
	p.Online = online
	return p, s.repo.UpdatePump(ctx, p)
}

// PumpQuote is a pump plus the current price of every grade, used by the
// fueling service to validate a fill-up and snapshot its price.
type PumpQuote struct {
	Pump   Pump                  `json:"pump"`
	Prices map[common.Fuel]int64 `json:"prices"`
}

// Quote returns a pump with current prices.
func (s *Service) Quote(ctx context.Context, stationID string, n int) (PumpQuote, error) {
	p, err := s.repo.GetPump(ctx, stationID, n)
	if err != nil {
		return PumpQuote{}, mapNotFound(err)
	}
	prices, err := s.repo.Prices(ctx)
	return PumpQuote{Pump: p, Prices: prices}, err
}

// Prices returns the price per litre of each grade.
func (s *Service) Prices(ctx context.Context) (map[common.Fuel]int64, error) {
	return s.repo.Prices(ctx)
}

// StaffList lists a station's staff.
func (s *Service) StaffList(ctx context.Context, stationID string) ([]Staff, error) {
	return s.repo.ListStaff(ctx, stationID)
}

// StaffMember returns one staff member of stationID.
func (s *Service) StaffMember(ctx context.Context, stationID, id string) (Staff, error) {
	st, err := s.repo.GetStaff(ctx, id)
	if err != nil || st.StationID != stationID {
		return Staff{}, errNotFound
	}
	return st, nil
}

// StaffInput creates or replaces a staff member.
type StaffInput struct {
	Name   i18n.Text `json:"name"`
	Code   string    `json:"code"`
	Role   StaffRole `json:"role"`
	Shift  string    `json:"shift"`
	Device string    `json:"device"`
	Active bool      `json:"active"`
}

// SaveStaff creates (id == "") or updates a staff member of stationID.
func (s *Service) SaveStaff(ctx context.Context, stationID, id string, in StaffInput) (Staff, error) {
	in.Name, in.Code, in.Device = in.Name.Trimmed(), strings.TrimSpace(in.Code), strings.TrimSpace(in.Device)
	switch {
	case !in.Name.Complete():
		return Staff{}, fieldErr("الاسم", "Name")
	case in.Code == "":
		return Staff{}, fieldErr("الرقم الوظيفي", "Staff ID")
	case in.Role != RoleSupervisor && in.Role != RoleAttendant:
		return Staff{}, fieldErr("الدور", "Role")
	case !slices.Contains([]string{"A", "B", "C"}, in.Shift):
		return Staff{}, fieldErr("الوردية", "Shift")
	}
	st := Staff{StationID: stationID}
	if id != "" {
		cur, err := s.StaffMember(ctx, stationID, id)
		if err != nil {
			return Staff{}, err
		}
		st = cur
	}
	others, err := s.repo.ListStaff(ctx, stationID)
	if err != nil {
		return Staff{}, err
	}
	if slices.ContainsFunc(others, func(o Staff) bool { return o.ID != id && strings.EqualFold(o.Code, in.Code) }) {
		return Staff{}, errDupCode
	}
	st.Name, st.Code, st.Role, st.Shift, st.Device, st.Active = in.Name, in.Code, in.Role, in.Shift, in.Device, in.Active
	if id == "" {
		newID, err := s.repo.InsertStaff(ctx, st)
		if err != nil {
			return Staff{}, err
		}
		st.ID = newID
		return st, nil
	}
	return st, s.repo.UpdateStaff(ctx, st)
}

// SetStaffActive suspends or reactivates a staff member.
func (s *Service) SetStaffActive(ctx context.Context, stationID, id string, active bool) (Staff, error) {
	st, err := s.StaffMember(ctx, stationID, id)
	if err != nil {
		return Staff{}, err
	}
	st.Active = active
	return st, s.repo.UpdateStaff(ctx, st)
}
