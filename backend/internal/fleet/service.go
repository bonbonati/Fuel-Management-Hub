package fleet

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/bonbonati/fuel-management-hub/backend/internal/common"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/i18n"
)

// ErrNotFound is returned by Repo getters when the row does not exist.
var ErrNotFound = errors.New("fleet: not found")

// Repo is the persistence port of the fleet service. Getters return
// ErrNotFound for missing rows. Inside Store.InTx, getters lock the row
// (SELECT … FOR UPDATE) so read-modify-write use cases are race free.
type Repo interface {
	GetOrg(ctx context.Context, id string) (Org, error)
	UpdateOrg(ctx context.Context, o Org) error

	ListTiers(ctx context.Context, orgID string) ([]Tier, error)
	GetTier(ctx context.Context, id string) (Tier, error)
	InsertTier(ctx context.Context, t Tier) (string, error)
	UpdateTier(ctx context.Context, t Tier) error

	ListDrivers(ctx context.Context, orgID string) ([]Driver, error)
	GetDriver(ctx context.Context, id string) (Driver, error)
	InsertDriver(ctx context.Context, d Driver) (string, error)
	UpdateDriver(ctx context.Context, d Driver) error

	ListVehicles(ctx context.Context, orgID string) ([]Vehicle, error)
	GetVehicle(ctx context.Context, id string) (Vehicle, error)
	InsertVehicle(ctx context.Context, v Vehicle) (string, error)
	UpdateVehicle(ctx context.Context, v Vehicle) error

	GetRules(ctx context.Context, orgID string) (Rules, error)
	UpdateRules(ctx context.Context, orgID string, r Rules) error

	ListRequests(ctx context.Context, orgID string) ([]Request, error) // newest first
	GetRequest(ctx context.Context, id string) (Request, error)
	InsertRequest(ctx context.Context, r Request) (Request, error) // returns id and createdAt
	UpdateRequest(ctx context.Context, r Request) error
}

// Store is a Repo that can also run a function in a transaction.
type Store interface {
	Repo
	InTx(ctx context.Context, fn func(Repo) error) error
}

// Service implements the fleet use cases. All methods are scoped to an org:
// rows of another org are reported as not found (tenant isolation).
type Service struct {
	store Store
	now   func() time.Time
}

// NewService returns a Service over store.
func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }

// mapNotFound converts the repo sentinel into the API error.
func mapNotFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return errNotFound
	}
	return err
}

// ---------- org ----------

// Org returns the caller's org.
func (s *Service) Org(ctx context.Context, orgID string) (Org, error) {
	o, err := s.store.GetOrg(ctx, orgID)
	return o, mapNotFound(err)
}

// OrgPatch updates the editable company details (blank fields are rejected).
type OrgPatch struct {
	Name  *i18n.Text `json:"name"`
	Phone *string    `json:"phone"`
}

// UpdateOrg applies p to the org.
func (s *Service) UpdateOrg(ctx context.Context, orgID string, p OrgPatch) (Org, error) {
	var out Org
	err := s.store.InTx(ctx, func(r Repo) error {
		o, err := r.GetOrg(ctx, orgID)
		if err != nil {
			return mapNotFound(err)
		}
		if p.Name != nil {
			n := p.Name.Trimmed()
			if !n.Complete() {
				return fieldErr("الاسم", "Name")
			}
			o.Name = n
		}
		if p.Phone != nil {
			ph := strings.TrimSpace(*p.Phone)
			if ph == "" {
				return fieldErr("هاتف العمليات", "Operations phone")
			}
			o.Phone = ph
		}
		out = o
		return r.UpdateOrg(ctx, o)
	})
	return out, err
}

// ---------- tiers ----------

// Tiers lists the org's tiers.
func (s *Service) Tiers(ctx context.Context, orgID string) ([]Tier, error) {
	return s.store.ListTiers(ctx, orgID)
}

// TierInput creates or replaces a tier and its vehicle assignment.
type TierInput struct {
	Name        i18n.Text `json:"name"`
	Budget      int64     `json:"budget"`
	DailyLitres int       `json:"dailyLitres"`
	Window      string    `json:"window"`
	VehicleIDs  []string  `json:"vehicleIds"` // exactly the vehicles that belong to the tier afterwards
}

// tierColors rotates UI colors for new tiers (same order as the prototype).
var tierColors = []string{"bl", "or", "vi", "em", "cy"}

// SaveTier creates (id == "") or updates a tier. Listed vehicles are moved
// into the tier; vehicles previously in it but not listed leave it.
func (s *Service) SaveTier(ctx context.Context, orgID, id string, in TierInput) (Tier, error) {
	in.Name = in.Name.Trimmed()
	in.Window = strings.TrimSpace(in.Window)
	switch {
	case !in.Name.Complete():
		return Tier{}, fieldErr("اسم الفئة", "Tier name")
	case in.Budget < common.MinFillAmount || in.Budget > common.MaxMoney:
		return Tier{}, fieldErr("الميزانية الشهرية لكل مركبة", "Monthly budget per vehicle")
	case in.DailyLitres < 1 || in.DailyLitres > 10_000:
		return Tier{}, fieldErr("الحد اليومي", "Daily cap")
	case in.Window == "":
		return Tier{}, fieldErr("ساعات التعبئة", "Fueling hours")
	}
	var out Tier
	err := s.store.InTx(ctx, func(r Repo) error {
		var t Tier
		if id == "" {
			existing, err := r.ListTiers(ctx, orgID)
			if err != nil {
				return err
			}
			t = Tier{OrgID: orgID, Icon: "layers", Color: tierColors[len(existing)%len(tierColors)]}
		} else {
			cur, err := r.GetTier(ctx, id)
			if err != nil || cur.OrgID != orgID {
				return errNotFound
			}
			t = cur
		}
		t.Name, t.Budget, t.DailyLitres, t.Window = in.Name, in.Budget, in.DailyLitres, in.Window
		if id == "" {
			newID, err := r.InsertTier(ctx, t)
			if err != nil {
				return err
			}
			t.ID = newID
		} else if err := r.UpdateTier(ctx, t); err != nil {
			return err
		}
		vehicles, err := r.ListVehicles(ctx, orgID)
		if err != nil {
			return err
		}
		for _, vid := range in.VehicleIDs {
			if !slices.ContainsFunc(vehicles, func(v Vehicle) bool { return v.ID == vid }) {
				return fieldErr("المركبات", "Vehicles")
			}
		}
		for _, v := range vehicles {
			in := slices.Contains(in.VehicleIDs, v.ID)
			was := v.TierID != nil && *v.TierID == t.ID
			if in == was {
				continue
			}
			if in {
				v.TierID = &t.ID
			} else {
				v.TierID = nil
			}
			if err := r.UpdateVehicle(ctx, v); err != nil {
				return err
			}
		}
		out = t
		return nil
	})
	return out, err
}

// ---------- drivers ----------

// Drivers lists the org's drivers.
func (s *Service) Drivers(ctx context.Context, orgID string) ([]Driver, error) {
	return s.store.ListDrivers(ctx, orgID)
}

// DriverInput creates or replaces a driver and their vehicle assignment.
type DriverInput struct {
	Name         i18n.Text `json:"name"`
	Code         string    `json:"code"`
	Phone        string    `json:"phone"`
	AppActivated bool      `json:"appActivated"`
	Active       bool      `json:"active"`
	VehicleID    *string   `json:"vehicleId"` // nil/"" = no vehicle
}

// SaveDriver creates (id == "") or updates a driver. A driver has at most one
// vehicle: assigning one unassigns any other vehicle they had, and the chosen
// vehicle's previous driver is unassigned from it.
func (s *Service) SaveDriver(ctx context.Context, orgID, id string, in DriverInput) (Driver, error) {
	in.Name = in.Name.Trimmed()
	in.Code = strings.TrimSpace(in.Code)
	if !in.Name.Complete() {
		return Driver{}, fieldErr("الاسم", "Name")
	}
	if in.Code == "" {
		return Driver{}, fieldErr("رقم السائق", "Driver ID")
	}
	phone, err := NormalizePhone(in.Phone)
	if err != nil {
		return Driver{}, err
	}
	var out Driver
	err = s.store.InTx(ctx, func(r Repo) error {
		d := Driver{OrgID: orgID}
		if id != "" {
			cur, err := r.GetDriver(ctx, id)
			if err != nil || cur.OrgID != orgID {
				return errNotFound
			}
			d = cur
		}
		d.Name, d.Code, d.Phone, d.AppActivated, d.Active = in.Name, in.Code, phone, in.AppActivated, in.Active
		if id == "" {
			newID, err := r.InsertDriver(ctx, d)
			if err != nil {
				return err
			}
			d.ID = newID
		} else if err := r.UpdateDriver(ctx, d); err != nil {
			return err
		}
		target := ""
		if in.VehicleID != nil {
			target = *in.VehicleID
		}
		if err := assignDriver(ctx, r, orgID, d.ID, target); err != nil {
			return err
		}
		out = d
		return nil
	})
	return out, err
}

// assignDriver makes vehicleID ("" for none) the only vehicle of driverID.
func assignDriver(ctx context.Context, r Repo, orgID, driverID, vehicleID string) error {
	vehicles, err := r.ListVehicles(ctx, orgID)
	if err != nil {
		return err
	}
	if vehicleID != "" && !slices.ContainsFunc(vehicles, func(v Vehicle) bool { return v.ID == vehicleID }) {
		return fieldErr("المركبة المخصصة", "Assigned vehicle")
	}
	// Unassign first so the unique (driver_id) index never sees two rows.
	for _, v := range vehicles {
		if v.DriverID != nil && *v.DriverID == driverID && v.ID != vehicleID {
			v.DriverID = nil
			if err := r.UpdateVehicle(ctx, v); err != nil {
				return err
			}
		}
	}
	for _, v := range vehicles {
		if v.ID == vehicleID && (v.DriverID == nil || *v.DriverID != driverID) {
			v.DriverID = &driverID
			return r.UpdateVehicle(ctx, v)
		}
	}
	return nil
}

// SetDriverActive suspends or reactivates a driver.
func (s *Service) SetDriverActive(ctx context.Context, orgID, id string, active bool) (Driver, error) {
	var out Driver
	err := s.store.InTx(ctx, func(r Repo) error {
		d, err := r.GetDriver(ctx, id)
		if err != nil || d.OrgID != orgID {
			return errNotFound
		}
		d.Active = active
		out = d
		return r.UpdateDriver(ctx, d)
	})
	return out, err
}

// ---------- vehicles ----------

// Vehicles lists the org's vehicles with computed limits.
func (s *Service) Vehicles(ctx context.Context, orgID string) ([]VehicleView, error) {
	vs, err := s.store.ListVehicles(ctx, orgID)
	if err != nil {
		return nil, err
	}
	tiers, err := s.store.ListTiers(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]VehicleView, len(vs))
	for i, v := range vs {
		out[i] = View(v, findTier(tiers, v.TierID))
	}
	return out, nil
}

func findTier(tiers []Tier, id *string) *Tier {
	if id == nil {
		return nil
	}
	for i := range tiers {
		if tiers[i].ID == *id {
			return &tiers[i]
		}
	}
	return nil
}

// VehicleInput creates or replaces a vehicle.
type VehicleInput struct {
	Plate       common.Plate `json:"plate"`
	Name        i18n.Text    `json:"name"`
	Fuel        common.Fuel  `json:"fuel"`
	TierID      *string      `json:"tierId"`
	CustomLimit *int64       `json:"customLimit"`
	DriverID    *string      `json:"driverId"`
	TagBattery  int          `json:"tagBattery"`
	Active      bool         `json:"active"`
}

// SaveVehicle creates (id == "") or updates a vehicle. New vehicles start
// with no approved extra. The tag battery is clamped to 0–100.
func (s *Service) SaveVehicle(ctx context.Context, orgID, id string, in VehicleInput) (VehicleView, error) {
	plate, ok := in.Plate.Normalize()
	if !ok {
		return VehicleView{}, fieldErr("أرقام اللوحة", "Plate number")
	}
	in.Name = in.Name.Trimmed()
	if !in.Name.Complete() {
		return VehicleView{}, fieldErr("الطراز", "Model")
	}
	if !in.Fuel.Valid() {
		return VehicleView{}, fieldErr("نوع الوقود", "Fuel grade")
	}
	if in.CustomLimit != nil && (*in.CustomLimit < 0 || *in.CustomLimit > common.MaxMoney) {
		return VehicleView{}, fieldErr("حد مخصص", "Custom limit")
	}
	var out VehicleView
	err := s.store.InTx(ctx, func(r Repo) error {
		v := Vehicle{OrgID: orgID}
		if id != "" {
			cur, err := r.GetVehicle(ctx, id)
			if err != nil || cur.OrgID != orgID {
				return errNotFound
			}
			v = cur
		}
		var tier *Tier
		if in.TierID != nil && *in.TierID != "" {
			t, err := r.GetTier(ctx, *in.TierID)
			if err != nil || t.OrgID != orgID {
				return fieldErr("الفئة", "Tier")
			}
			tier = &t
			v.TierID = &t.ID
		} else {
			v.TierID = nil
		}
		v.Plate, v.Name, v.Fuel, v.CustomLimit, v.Active = plate, in.Name, in.Fuel, in.CustomLimit, in.Active
		v.TagBattery = max(0, min(100, in.TagBattery))
		newDriver := ""
		if in.DriverID != nil && *in.DriverID != "" {
			d, err := r.GetDriver(ctx, *in.DriverID)
			if err != nil || d.OrgID != orgID {
				return fieldErr("السائق", "Driver")
			}
			newDriver = d.ID
		}
		// The driver is (re)assigned through assignDriver so the
		// one-vehicle-per-driver invariant holds; keep the old one meanwhile.
		if id == "" {
			v.DriverID = nil
			newID, err := r.InsertVehicle(ctx, v)
			if err != nil {
				return err
			}
			v.ID = newID
		} else {
			if newDriver == "" {
				v.DriverID = nil
			}
			if err := r.UpdateVehicle(ctx, v); err != nil {
				return err
			}
		}
		if newDriver != "" {
			if err := assignDriver(ctx, r, orgID, newDriver, v.ID); err != nil {
				return err
			}
			v.DriverID = &newDriver
		}
		out = View(v, tier)
		return nil
	})
	return out, err
}

// updateVehicle loads a vehicle of orgID, applies fn and saves it.
func (s *Service) updateVehicle(ctx context.Context, orgID, id string, fn func(r Repo, v *Vehicle) error) (VehicleView, error) {
	var out VehicleView
	err := s.store.InTx(ctx, func(r Repo) error {
		v, err := r.GetVehicle(ctx, id)
		if err != nil || v.OrgID != orgID {
			return errNotFound
		}
		if err := fn(r, &v); err != nil {
			return err
		}
		if err := r.UpdateVehicle(ctx, v); err != nil {
			return err
		}
		var tier *Tier
		if v.TierID != nil {
			if t, err := r.GetTier(ctx, *v.TierID); err == nil {
				tier = &t
			}
		}
		out = View(v, tier)
		return nil
	})
	return out, err
}

// SetVehicleActive suspends or reactivates a vehicle.
func (s *Service) SetVehicleActive(ctx context.Context, orgID, id string, active bool) (VehicleView, error) {
	return s.updateVehicle(ctx, orgID, id, func(_ Repo, v *Vehicle) error { v.Active = active; return nil })
}

// SetVehicleTier moves a vehicle into tierID ("" removes it from its tier).
func (s *Service) SetVehicleTier(ctx context.Context, orgID, id, tierID string) (VehicleView, error) {
	return s.updateVehicle(ctx, orgID, id, func(r Repo, v *Vehicle) error {
		if tierID == "" {
			v.TierID = nil
			return nil
		}
		t, err := r.GetTier(ctx, tierID)
		if err != nil || t.OrgID != orgID {
			return fieldErr("الفئة", "Tier")
		}
		v.TierID = &t.ID
		return nil
	})
}

// IncreaseLimit adds amount to the vehicle's approved extra for this month
// (the "Add SDG 300,000" alert action).
func (s *Service) IncreaseLimit(ctx context.Context, orgID, id string, amount int64) (VehicleView, error) {
	if amount <= 0 || amount > common.MaxMoney {
		return VehicleView{}, errAmount
	}
	return s.updateVehicle(ctx, orgID, id, func(_ Repo, v *Vehicle) error { v.Extra += amount; return nil })
}

// ---------- rules ----------

// Rules returns the org's rules.
func (s *Service) Rules(ctx context.Context, orgID string) (Rules, error) {
	r, err := s.store.GetRules(ctx, orgID)
	return r, mapNotFound(err)
}

// RulesPatch toggles individual rules; nil fields are left unchanged.
type RulesPatch struct {
	Network           *bool    `json:"network"`
	AntiSiphon        *bool    `json:"antiSiphon"`
	Geofence          *bool    `json:"geofence"`
	WrongFuel         *bool    `json:"wrongFuel"`
	Odometer          *bool    `json:"odometer"`
	MaxFill           *bool    `json:"maxFill"`
	MaxFillAmount     *int64   `json:"maxFillAmount"`
	Hours             *bool    `json:"hours"`
	NetworkStationIDs []string `json:"networkStationIds"`
}

// PatchRules applies p to the org's rules.
func (s *Service) PatchRules(ctx context.Context, orgID string, p RulesPatch) (Rules, error) {
	if p.MaxFillAmount != nil && (*p.MaxFillAmount < common.MinFillAmount || *p.MaxFillAmount > common.MaxMoney) {
		return Rules{}, errMaxFill
	}
	var out Rules
	err := s.store.InTx(ctx, func(r Repo) error {
		cur, err := r.GetRules(ctx, orgID)
		if err != nil {
			return mapNotFound(err)
		}
		set := func(dst *bool, v *bool) {
			if v != nil {
				*dst = *v
			}
		}
		set(&cur.Network, p.Network)
		set(&cur.AntiSiphon, p.AntiSiphon)
		set(&cur.Geofence, p.Geofence)
		set(&cur.WrongFuel, p.WrongFuel)
		set(&cur.Odometer, p.Odometer)
		set(&cur.MaxFill, p.MaxFill)
		set(&cur.Hours, p.Hours)
		if p.MaxFillAmount != nil {
			cur.MaxFillAmount = *p.MaxFillAmount
		}
		if p.NetworkStationIDs != nil {
			cur.NetworkStationIDs = p.NetworkStationIDs
		}
		out = cur
		return r.UpdateRules(ctx, orgID, cur)
	})
	return out, err
}

// ---------- requests ----------

// Requests lists the org's requests (newest first), optionally filtered by
// status ("" or "all" for every status) and/or driver.
func (s *Service) Requests(ctx context.Context, orgID, status, driverID string) ([]Request, error) {
	all, err := s.store.ListRequests(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]Request, 0, len(all))
	for _, r := range all {
		if (status == "" || status == "all" || string(r.Status) == status) && (driverID == "" || r.DriverID == driverID) {
			out = append(out, r)
		}
	}
	return out, nil
}

// CreateRequest files a driver's request after validating it (see
// ValidateRequest). The driver's current vehicle is attached.
func (s *Service) CreateRequest(ctx context.Context, orgID, driverID string, in RequestInput) (Request, error) {
	in.Note = strings.TrimSpace(in.Note)
	var out Request
	err := s.store.InTx(ctx, func(r Repo) error {
		c, err := s.contextFor(ctx, r, driverID, "")
		if err != nil {
			return err
		}
		if c.OrgID != orgID {
			return errNotFound
		}
		all, err := r.ListRequests(ctx, orgID)
		if err != nil {
			return err
		}
		mine := slices.DeleteFunc(all, func(x Request) bool { return x.DriverID != driverID })
		if err := ValidateRequest(in, c.Block, mine); err != nil {
			return err
		}
		req := Request{OrgID: orgID, DriverID: driverID, Type: in.Type, Reason: in.Reason, Note: i18n.Same(in.Note), Status: StatusPending}
		if in.Type == RequestLimit {
			req.Amount = in.Amount
		}
		if c.Vehicle != nil {
			req.VehicleID = &c.Vehicle.ID
		}
		out, err = r.InsertRequest(ctx, req)
		return err
	})
	return out, err
}

// DecisionInput is a fleet manager's decision on a request.
type DecisionInput struct {
	Approve bool   `json:"approve"`
	Amount  *int64 `json:"amount"` // approved amount for limit requests (defaults to requested)
	Note    string `json:"note"`   // optional reply to the driver
}

// Decide approves or rejects a pending request:
//   - approving a limit request adds the approved amount to the vehicle's
//     extra (minimum 10,000 SDG; a vehicle must be linked);
//   - approving a reactivation reactivates both the driver and the vehicle.
func (s *Service) Decide(ctx context.Context, orgID, id string, in DecisionInput) (Request, error) {
	var out Request
	err := s.store.InTx(ctx, func(r Repo) error {
		req, err := r.GetRequest(ctx, id)
		if err != nil || req.OrgID != orgID {
			return errNotFound
		}
		if req.Status != StatusPending {
			return errNotPending
		}
		if in.Approve {
			switch req.Type {
			case RequestLimit:
				amount := req.Amount
				if in.Amount != nil {
					amount = *in.Amount
				}
				if amount < common.MinFillAmount || amount > common.MaxMoney {
					return errApprovedAmount
				}
				if req.VehicleID == nil {
					return errNoVehicleOnRequest
				}
				v, err := r.GetVehicle(ctx, *req.VehicleID)
				if err != nil {
					return errNoVehicleOnRequest
				}
				v.Extra += amount
				if err := r.UpdateVehicle(ctx, v); err != nil {
					return err
				}
				req.Approved = amount
			case RequestActivate:
				if d, err := r.GetDriver(ctx, req.DriverID); err == nil {
					d.Active = true
					if err := r.UpdateDriver(ctx, d); err != nil {
						return err
					}
				}
				if req.VehicleID != nil {
					if v, err := r.GetVehicle(ctx, *req.VehicleID); err == nil {
						v.Active = true
						if err := r.UpdateVehicle(ctx, v); err != nil {
							return err
						}
					}
				}
			}
			req.Status = StatusApproved
		} else {
			req.Status = StatusRejected
		}
		now := s.now()
		req.DecidedAt = &now
		req.Reply = i18n.Same(strings.TrimSpace(in.Note))
		out = req
		return r.UpdateRequest(ctx, req)
	})
	return out, err
}

// ---------- authorization contexts ----------

// DriverContext returns the authorization context of a driver.
func (s *Service) DriverContext(ctx context.Context, driverID string) (Context, error) {
	return s.contextFor(ctx, s.store, driverID, "")
}

// VehicleContext returns the authorization context of a vehicle (and its
// assigned driver, if any).
func (s *Service) VehicleContext(ctx context.Context, vehicleID string) (Context, error) {
	return s.contextFor(ctx, s.store, "", vehicleID)
}

// contextFor builds a Context from either a driver id or a vehicle id.
func (s *Service) contextFor(ctx context.Context, r Repo, driverID, vehicleID string) (Context, error) {
	var (
		d *Driver
		v *Vehicle
	)
	var orgID string
	if driverID != "" {
		dr, err := r.GetDriver(ctx, driverID)
		if err != nil {
			return Context{}, mapNotFound(err)
		}
		d, orgID = &dr, dr.OrgID
		vs, err := r.ListVehicles(ctx, orgID)
		if err != nil {
			return Context{}, err
		}
		for i := range vs {
			if vs[i].DriverID != nil && *vs[i].DriverID == driverID {
				v = &vs[i]
				break
			}
		}
	} else {
		vh, err := r.GetVehicle(ctx, vehicleID)
		if err != nil {
			return Context{}, mapNotFound(err)
		}
		v, orgID = &vh, vh.OrgID
		if vh.DriverID != nil {
			if dr, err := r.GetDriver(ctx, *vh.DriverID); err == nil {
				d = &dr
			}
		}
	}
	org, err := r.GetOrg(ctx, orgID)
	if err != nil {
		return Context{}, mapNotFound(err)
	}
	rules, err := r.GetRules(ctx, orgID)
	if err != nil {
		return Context{}, mapNotFound(err)
	}
	c := Context{OrgID: orgID, OrgName: org.Name, OrgPhone: org.Phone, Driver: d, Rules: rules, Block: ComputeBlock(d, v)}
	if v != nil {
		if v.TierID != nil {
			if t, err := r.GetTier(ctx, *v.TierID); err == nil {
				c.Tier = &t
			}
		}
		view := View(*v, c.Tier)
		c.Vehicle = &view
	}
	return c, nil
}
