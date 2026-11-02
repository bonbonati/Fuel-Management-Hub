// Package fleet is the fleet service: a fleet company (org), its vehicle
// tiers and budgets, drivers, vehicles, fuel rules and the drivers'
// limit-increase / reactivation requests.
//
// The service is the source of truth for every vehicle's monthly *limit*
// (tier budget or custom limit, plus approved increases). How much of the
// limit is *used* is owned by the payment service's ledger; clients and the
// fueling service combine the two.
//
// Layout: model.go (types), rules.go (pure business rules), service.go
// (use cases over the Repo interface), store_pg.go (Postgres Repo),
// http.go (REST API), seed.sql + migrations/ (schema and demo data).
package fleet

import (
	"time"

	"github.com/bonbonati/fuel-management-hub/backend/internal/common"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/i18n"
)

// Org is a fleet company account.
type Org struct {
	ID    string    `json:"id"`
	Name  i18n.Text `json:"name"`
	Phone string    `json:"phone"` // operations desk, shown to drivers
	City  i18n.Text `json:"city"`
}

// Tier groups vehicles that share a monthly per-vehicle budget, a daily
// litre cap and a fueling time window.
type Tier struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"orgId"`
	Icon        string    `json:"icon"`  // UI icon key (truck, bus, car, layers…)
	Color       string    `json:"color"` // UI color token (bl, or, vi, em, cy)
	Name        i18n.Text `json:"name"`
	Budget      int64     `json:"budget"` // SDG per vehicle per month
	DailyLitres int       `json:"dailyLitres"`
	Window      string    `json:"window"` // e.g. "06:00–21:00" or "24/7"
}

// Driver is a person allowed to fuel the vehicle assigned to them.
type Driver struct {
	ID           string    `json:"id"`
	OrgID        string    `json:"orgId"`
	Name         i18n.Text `json:"name"`
	Phone        string    `json:"phone"` // normalized "+249 XX XXX XXXX"
	Code         string    `json:"code"`  // staff number, e.g. "D-3071"; unique per org
	AppActivated bool      `json:"appActivated"`
	Active       bool      `json:"active"` // suspended drivers cannot use the fuel pass
}

// Vehicle is a fleet vehicle fitted with an RFID fuel tag.
type Vehicle struct {
	ID          string       `json:"id"`
	OrgID       string       `json:"orgId"`
	Plate       common.Plate `json:"plate"`
	Name        i18n.Text    `json:"name"` // make/model
	DriverID    *string      `json:"driverId"`
	Fuel        common.Fuel  `json:"fuel"` // the only grade it may take when wrongFuel is on
	TierID      *string      `json:"tierId"`
	CustomLimit *int64       `json:"customLimit"` // overrides the tier budget when set
	Extra       int64        `json:"extra"`       // approved increases this month
	Active      bool         `json:"active"`
	TagBattery  int          `json:"tagBattery"` // RFID tag battery %, 0–100
}

// VehicleView is a Vehicle with its computed limits, as returned by the API.
type VehicleView struct {
	Vehicle
	BaseLimit int64 `json:"baseLimit"` // custom limit or tier budget
	Limit     int64 `json:"limit"`     // BaseLimit + Extra
}

// Rules are the org's fueling policies and anti-fraud controls.
type Rules struct {
	Network           bool     `json:"network"`    // fuel only at network stations
	AntiSiphon        bool     `json:"antiSiphon"` // max one fill-up per 4h (not yet enforced)
	Geofence          bool     `json:"geofence"`   // vehicle within 50 m of pump (not yet enforced)
	WrongFuel         bool     `json:"wrongFuel"`  // only the vehicle's registered grade
	Odometer          bool     `json:"odometer"`   // odometer reading before each fill-up
	MaxFill           bool     `json:"maxFill"`    // cap a single fill-up at MaxFillAmount
	MaxFillAmount     int64    `json:"maxFillAmount"`
	Hours             bool     `json:"hours"` // tier time windows (not yet enforced)
	NetworkStationIDs []string `json:"networkStationIds"`
}

// RequestType is what a driver asks the fleet manager for.
type RequestType string

const (
	RequestLimit    RequestType = "limit"    // monthly limit increase
	RequestActivate RequestType = "activate" // reactivate a suspended driver/vehicle
)

// RequestStatus is the review state of a request.
type RequestStatus string

const (
	StatusPending  RequestStatus = "pending"
	StatusApproved RequestStatus = "approved"
	StatusRejected RequestStatus = "rejected"
)

// Request is a driver's request awaiting or after a fleet manager decision.
type Request struct {
	ID        string        `json:"id"` // e.g. "R-1044"
	OrgID     string        `json:"orgId"`
	DriverID  string        `json:"driverId"`
	VehicleID *string       `json:"vehicleId"`
	Type      RequestType   `json:"type"`
	Amount    int64         `json:"amount"` // requested increase (limit requests)
	Reason    string        `json:"reason"` // trip | extra | emergency | review | other
	Note      i18n.Text     `json:"note"`
	Status    RequestStatus `json:"status"`
	Approved  int64         `json:"approved"` // approved increase (limit requests)
	Reply     i18n.Text     `json:"reply"`    // manager's note to the driver
	CreatedAt time.Time     `json:"createdAt"`
	DecidedAt *time.Time    `json:"decidedAt"`
}

// BlockKind explains why a driver cannot fuel right now.
type BlockKind string

const (
	BlockDriver    BlockKind = "driver" // driver account suspended
	BlockNoVehicle BlockKind = "noveh"  // no vehicle assigned
	BlockVehicle   BlockKind = "veh"    // assigned vehicle suspended
)

// Block is returned when a driver/vehicle may not fuel.
type Block struct {
	Kind    BlockKind `json:"kind"`
	Message i18n.Text `json:"message"`
}

// Context is everything needed to authorize a fill-up for a driver or a
// vehicle. It is served to the fueling service and to the driver app (/me).
type Context struct {
	OrgID    string       `json:"orgId"`
	OrgName  i18n.Text    `json:"orgName"`
	OrgPhone string       `json:"orgPhone"`
	Driver   *Driver      `json:"driver"`
	Vehicle  *VehicleView `json:"vehicle"`
	Tier     *Tier        `json:"tier"`
	Rules    Rules        `json:"rules"`
	Block    *Block       `json:"block"`
}
