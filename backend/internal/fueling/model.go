// Package fueling is the fueling service: it orchestrates every fill-up.
//
// It owns the fill-up transactions and their lifecycle
// (auth → fueling → done), the drivers' rotating QR pass tokens and the
// attendants' scan sessions (both ephemeral, in Valkey), the pre-fueling
// odometer readings, and the fleet security log.
//
// To authorize a fill-up it asks the fleet service for the driver/vehicle
// context (limits, rules, suspension), the station service for the pump and
// prices, and the payment service for the wallet balance and usage; then it
// charges the payment service and records the transaction. A completed
// fill-up emits "fueling.transaction.completed" through the outbox.
package fueling

import (
	"time"

	"github.com/bonbonati/fuel-management-hub/backend/internal/common"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/i18n"
)

// Status of a transaction.
type Status string

const (
	StatusAuthorized Status = "auth"    // charged; waiting for the attendant to start the pump
	StatusFueling    Status = "fueling" // pump running
	StatusDone       Status = "done"    // completed, receipt issued
)

// Source is how the vehicle was identified.
type Source string

const (
	SourceApp  Source = "app"  // driver authorized in the app (4-digit code)
	SourceQR   Source = "qr"   // attendant scanned the driver's pass
	SourceRFID Source = "rfid" // attendant read the vehicle's fuel tag
)

// Transaction is one fill-up.
type Transaction struct {
	ID            string       `json:"id"`        // e.g. "T-90413"
	OrgID         string       `json:"orgId"`     // "" for individual customers
	VehicleID     string       `json:"vehicleId"` // "" for individual customers
	DriverID      string       `json:"driverId"`
	Plate         common.Plate `json:"plate"` // snapshot at fill-up time
	StationID     string       `json:"stationId"`
	Pump          int          `json:"pump"`
	Fuel          common.Fuel  `json:"fuel"`
	Amount        int64        `json:"amount"`        // SDG, VAT inclusive
	PricePerLitre int64        `json:"pricePerLitre"` // snapshot
	Litres        float64      `json:"litres"`        // Amount / PricePerLitre
	Status        Status       `json:"status"`
	Code          string       `json:"code,omitempty"` // 4-digit start code (app authorizations)
	Source        Source       `json:"source"`
	OdometerKm    *int         `json:"odometerKm"`
	CreatedAt     time.Time    `json:"createdAt"`
	StartedAt     *time.Time   `json:"startedAt"`
	CompletedAt   *time.Time   `json:"completedAt"`
}

// withLitres fills in the derived litres.
func (t Transaction) withLitres() Transaction {
	if t.PricePerLitre > 0 {
		t.Litres = float64(t.Amount) / float64(t.PricePerLitre)
	}
	return t
}

// Severity of a security event.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
)

// SecurityEvent is an entry of a fleet's security log (blocked attempts).
type SecurityEvent struct {
	ID        int64     `json:"id"`
	OrgID     string    `json:"orgId"`
	Severity  Severity  `json:"severity"`
	Title     i18n.Text `json:"title"`
	Detail    i18n.Text `json:"detail"`
	Ref       string    `json:"ref"` // driver code, plate or tag reference
	CreatedAt time.Time `json:"createdAt"`
}

// OdometerReading is a driver's pre-fueling odometer check.
type OdometerReading struct {
	DriverID   string    `json:"driverId"`
	Km         int       `json:"km"`
	RecordedAt time.Time `json:"recordedAt"`
}

// ---------- upstream contracts (mirrors of other services' responses) ----------
// The fueling service declares only the fields it needs, so the services stay
// independently deployable: extra fields upstream are simply ignored.

// FleetRules mirrors the fleet service's rules.
type FleetRules struct {
	Network           bool     `json:"network"`
	WrongFuel         bool     `json:"wrongFuel"`
	Odometer          bool     `json:"odometer"`
	MaxFill           bool     `json:"maxFill"`
	MaxFillAmount     int64    `json:"maxFillAmount"`
	NetworkStationIDs []string `json:"networkStationIds"`
}

// FleetDriver mirrors the fleet service's driver.
type FleetDriver struct {
	ID     string    `json:"id"`
	Name   i18n.Text `json:"name"`
	Code   string    `json:"code"`
	Active bool      `json:"active"`
}

// FleetVehicle mirrors the fleet service's vehicle view.
type FleetVehicle struct {
	ID     string       `json:"id"`
	Name   i18n.Text    `json:"name"`
	Plate  common.Plate `json:"plate"`
	Fuel   common.Fuel  `json:"fuel"`
	Active bool         `json:"active"`
	Limit  int64        `json:"limit"`
}

// FleetBlock mirrors the fleet service's block.
type FleetBlock struct {
	Kind    string    `json:"kind"` // driver | noveh | veh
	Message i18n.Text `json:"message"`
}

// FleetContext mirrors the fleet service's authorization context.
type FleetContext struct {
	OrgID   string        `json:"orgId"`
	OrgName i18n.Text     `json:"orgName"`
	Driver  *FleetDriver  `json:"driver"`
	Vehicle *FleetVehicle `json:"vehicle"`
	Rules   FleetRules    `json:"rules"`
	Block   *FleetBlock   `json:"block"`
}

// Pump mirrors the station service's pump.
type Pump struct {
	StationID string        `json:"stationId"`
	Number    int           `json:"number"`
	Grades    []common.Fuel `json:"grades"`
	Online    bool          `json:"online"`
}

// PumpQuote mirrors the station service's pump quote.
type PumpQuote struct {
	Pump   Pump                  `json:"pump"`
	Prices map[common.Fuel]int64 `json:"prices"`
}

// StationInfo mirrors the fields of a station the fueling service uses.
type StationInfo struct {
	ID   string    `json:"id"`
	Name i18n.Text `json:"name"`
}

// Allowance mirrors the payment service's allowance.
type Allowance struct {
	Balance int64 `json:"balance"`
	Used    int64 `json:"used"`
}

// ChargeRequest mirrors the payment service's charge input.
type ChargeRequest struct {
	TransactionID string `json:"transactionId"`
	OrgID         string `json:"orgId"`
	VehicleID     string `json:"vehicleId"`
	Amount        int64  `json:"amount"`
	VehicleLimit  int64  `json:"vehicleLimit"`
}
