package fueling

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"math"
	"slices"
	"time"

	"github.com/bonbonati/fuel-management-hub/backend/internal/common"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/events"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/i18n"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/kv"
)

// ---------- ports ----------

// Fleet is the fleet service as seen by fueling.
type Fleet interface {
	DriverContext(ctx context.Context, driverID string) (FleetContext, error)
	VehicleContext(ctx context.Context, vehicleID string) (FleetContext, error)
}

// Stations is the station service as seen by fueling.
type Stations interface {
	Quote(ctx context.Context, stationID string, pump int) (PumpQuote, error)
	Station(ctx context.Context, stationID string) (StationInfo, error)
}

// Payments is the payment service as seen by fueling.
type Payments interface {
	Allowance(ctx context.Context, orgID, vehicleID string) (Allowance, error)
	Charge(ctx context.Context, in ChargeRequest) error
	Refund(ctx context.Context, txID string) error
}

// ErrNotFound is returned by Repo getters for missing rows.
var ErrNotFound = errors.New("fueling: not found")

// TxFilter scopes a transaction listing. Exactly one field is set.
type TxFilter struct {
	DriverID, OrgID, StationID string
}

// Repo is the persistence port of the fueling service.
type Repo interface {
	NextTxID(ctx context.Context) (string, error)
	InsertTx(ctx context.Context, t Transaction) error
	GetTx(ctx context.Context, id string) (Transaction, error) // locks the row inside InTx
	UpdateTx(ctx context.Context, t Transaction) error
	ListTx(ctx context.Context, f TxFilter, limit int) ([]Transaction, error)
	FindOpenByCode(ctx context.Context, stationID, code string) (Transaction, error)
	CodeInUse(ctx context.Context, stationID, code string) (bool, error)
	PumpBusy(ctx context.Context, stationID string, pump int) (bool, error)
	PendingOdometer(ctx context.Context, driverID string) (*OdometerReading, error)
	InsertOdometer(ctx context.Context, r OdometerReading) error
	ConsumeOdometer(ctx context.Context, driverID, txID string) error
	InsertSecurityEvent(ctx context.Context, e SecurityEvent) error
	ListSecurityEvents(ctx context.Context, orgID string, limit int) ([]SecurityEvent, error)
	Enqueue(ctx context.Context, ev events.Event) error // outbox; only meaningful inside InTx
}

// Store is a Repo that can run a function in a transaction.
type Store interface {
	Repo
	InTx(ctx context.Context, fn func(Repo) error) error
}

// ---------- service ----------

// Service implements the fueling use cases.
type Service struct {
	store    Store
	kv       kv.Store
	fleet    Fleet
	stations Stations
	payments Payments
	now      func() time.Time
	passTTL  time.Duration
	scanTTL  time.Duration
}

// NewService wires the fueling service.
func NewService(store Store, cache kv.Store, f Fleet, s Stations, p Payments) *Service {
	return &Service{store: store, kv: cache, fleet: f, stations: s, payments: p, now: time.Now,
		passTTL: 30 * time.Second, scanTTL: 5 * time.Minute}
}

// passGrace keeps a token valid a little after the app shows it expired, so
// a scan taken at the last second still works.
const passGrace = 5 * time.Second

// passClaim is stored in Valkey under pass:<token>.
type passClaim struct {
	DriverID  string `json:"driverId"`
	VehicleID string `json:"vehicleId"`
	OrgID     string `json:"orgId"`
}

// scanSession is stored in Valkey under scan:<id> between scan and dispense.
type scanSession struct {
	ID        string `json:"id"`
	Source    Source `json:"source"`
	Token     string `json:"token,omitempty"`
	VehicleID string `json:"vehicleId"`
	StationID string `json:"stationId"`
}

// ---------- driver: pass status, odometer, token ----------

// AllowanceView is a vehicle's monthly allowance as shown in the driver app.
type AllowanceView struct {
	VehicleID string `json:"vehicleId"`
	Limit     int64  `json:"limit"`
	Used      int64  `json:"used"`
	Remaining int64  `json:"remaining"` // max(0, limit - used)
	Available int64  `json:"available"` // what may be fueled now (Cap)
	Reason    string `json:"reason"`    // limit | perfill
}

// OdometerStatus is the pre-fueling check state.
type OdometerStatus struct {
	Required bool `json:"required"`
	Recorded bool `json:"recorded"`
	Km       *int `json:"km"`
}

// PassStatus is the driver's fuel pass state.
type PassStatus struct {
	Allowance *AllowanceView `json:"allowance"`
	Odometer  OdometerStatus `json:"odometer"`
	Block     *FleetBlock    `json:"block"`
}

func (s *Service) allowanceView(ctx context.Context, fc FleetContext) (*AllowanceView, error) {
	if fc.Vehicle == nil {
		return nil, nil
	}
	al, err := s.payments.Allowance(ctx, fc.OrgID, fc.Vehicle.ID)
	if err != nil {
		return nil, err
	}
	c, reason := Cap(fc.Vehicle.Limit, al.Used, al.Balance, fc.Rules)
	return &AllowanceView{VehicleID: fc.Vehicle.ID, Limit: fc.Vehicle.Limit, Used: al.Used,
		Remaining: max(0, fc.Vehicle.Limit-al.Used), Available: c, Reason: reason}, nil
}

// PassStatus returns the calling driver's allowance, odometer check and block.
func (s *Service) PassStatus(ctx context.Context, driverID string) (PassStatus, error) {
	fc, err := s.fleet.DriverContext(ctx, driverID)
	if err != nil {
		return PassStatus{}, err
	}
	al, err := s.allowanceView(ctx, fc)
	if err != nil {
		return PassStatus{}, err
	}
	odo, err := s.store.PendingOdometer(ctx, driverID)
	if err != nil {
		return PassStatus{}, err
	}
	st := PassStatus{Allowance: al, Block: fc.Block, Odometer: OdometerStatus{Required: fc.Rules.Odometer, Recorded: odo != nil}}
	if odo != nil {
		st.Odometer.Km = &odo.Km
	}
	return st, nil
}

// RecordOdometer stores the driver's pre-fueling odometer reading. It is
// attached to (and consumed by) the driver's next fill-up.
func (s *Service) RecordOdometer(ctx context.Context, driverID, km string) (OdometerStatus, error) {
	n, err := ParseOdometer(km)
	if err != nil {
		return OdometerStatus{}, err
	}
	if err := s.store.InsertOdometer(ctx, OdometerReading{DriverID: driverID, Km: n, RecordedAt: s.now()}); err != nil {
		return OdometerStatus{}, err
	}
	return OdometerStatus{Required: true, Recorded: true, Km: &n}, nil
}

// PassToken is a freshly issued QR pass token.
type PassToken struct {
	Token      string    `json:"token"`
	ExpiresAt  time.Time `json:"expiresAt"`
	TTLSeconds int       `json:"ttlSeconds"`
}

// IssuePass creates a rotating QR pass token for the driver's vehicle.
// Tokens are issued even while the driver is suspended, so that a scan of a
// suspended driver's pass can be logged as a security event.
func (s *Service) IssuePass(ctx context.Context, driverID string) (PassToken, error) {
	fc, err := s.fleet.DriverContext(ctx, driverID)
	if err != nil {
		return PassToken{}, err
	}
	if fc.Vehicle == nil || fc.Driver == nil {
		return PassToken{}, errNoVehicle
	}
	claim := passClaim{DriverID: driverID, VehicleID: fc.Vehicle.ID, OrgID: fc.OrgID}
	for range 5 { // collisions are astronomically unlikely; NX makes them harmless
		tok := NewPassToken(fc.Driver.Code, fc.Vehicle.Fuel)
		ok, err := s.kv.SetJSON(ctx, "pass:"+tok, claim, s.passTTL+passGrace, true)
		if err != nil {
			return PassToken{}, err
		}
		if ok {
			return PassToken{Token: tok, ExpiresAt: s.now().Add(s.passTTL), TTLSeconds: int(s.passTTL / time.Second)}, nil
		}
	}
	return PassToken{}, httpx.ErrUnavailable
}

// ---------- driver: app authorization ----------

// AuthorizeInput is the driver's pay-sheet confirmation.
type AuthorizeInput struct {
	StationID string      `json:"stationId"`
	Pump      int         `json:"pump"`
	Fuel      common.Fuel `json:"fuel"`
	Amount    int64       `json:"amount"`
}

// Authorize charges the company account and creates an "auth" transaction
// with a 4-digit start code. Checks, in order: driver/vehicle not suspended,
// odometer recorded (if required), station in network, fuel grade allowed,
// minimum amount, pump online with that grade, amount within the cap.
func (s *Service) Authorize(ctx context.Context, driverID string, in AuthorizeInput) (Transaction, error) {
	fc, err := s.fleet.DriverContext(ctx, driverID)
	if err != nil {
		return Transaction{}, err
	}
	if fc.Block != nil {
		return Transaction{}, blockError(fc.Block)
	}
	odo, err := s.store.PendingOdometer(ctx, driverID)
	if err != nil {
		return Transaction{}, err
	}
	if fc.Rules.Odometer && odo == nil {
		return Transaction{}, errOdometerNeeded
	}
	if !InNetwork(fc.Rules, in.StationID) {
		return Transaction{}, errNotInNetwork
	}
	if !in.Fuel.Valid() {
		return Transaction{}, errBadFuel
	}
	if fc.Rules.WrongFuel && in.Fuel != fc.Vehicle.Fuel {
		return Transaction{}, errWrongFuel
	}
	if in.Amount < common.MinFillAmount {
		return Transaction{}, errMinAmount
	}
	q, err := s.stations.Quote(ctx, in.StationID, in.Pump)
	if err != nil {
		return Transaction{}, err
	}
	if !q.Pump.Online || !slices.Contains(q.Pump.Grades, in.Fuel) {
		return Transaction{}, errPumpOffline
	}
	al, err := s.payments.Allowance(ctx, fc.OrgID, fc.Vehicle.ID)
	if err != nil {
		return Transaction{}, err
	}
	if c, reason := Cap(fc.Vehicle.Limit, al.Used, al.Balance, fc.Rules); in.Amount > c {
		return Transaction{}, capError(c, reason)
	}
	code, err := s.freeCode(ctx, in.StationID)
	if err != nil {
		return Transaction{}, err
	}
	t := Transaction{
		OrgID: fc.OrgID, VehicleID: fc.Vehicle.ID, DriverID: driverID, Plate: fc.Vehicle.Plate,
		StationID: in.StationID, Pump: in.Pump, Fuel: in.Fuel, Amount: in.Amount, PricePerLitre: q.Prices[in.Fuel],
		Status: StatusAuthorized, Code: code, Source: SourceApp, CreatedAt: s.now(),
	}
	if odo != nil {
		t.OdometerKm = &odo.Km
	}
	return s.chargeAndRecord(ctx, t, fc.Vehicle.Limit, odo != nil)
}

// freeCode picks a start code not used by another open authorization at the station.
func (s *Service) freeCode(ctx context.Context, stationID string) (string, error) {
	for range 20 {
		c := StartCode()
		used, err := s.store.CodeInUse(ctx, stationID, c)
		if err != nil {
			return "", err
		}
		if !used {
			return c, nil
		}
	}
	return "", httpx.ErrUnavailable.WithDetail("no free start code")
}

// chargeAndRecord charges the payment service, then records the transaction
// (and consumes the odometer reading) in one database transaction. If
// recording fails after a successful charge, the charge is refunded
// (compensating action); the charge is idempotent by transaction id.
func (s *Service) chargeAndRecord(ctx context.Context, t Transaction, limit int64, consumeOdo bool) (Transaction, error) {
	id, err := s.store.NextTxID(ctx)
	if err != nil {
		return Transaction{}, err
	}
	t.ID = id
	if err := s.payments.Charge(ctx, ChargeRequest{TransactionID: id, OrgID: t.OrgID, VehicleID: t.VehicleID, Amount: t.Amount, VehicleLimit: limit}); err != nil {
		return Transaction{}, err
	}
	err = s.store.InTx(ctx, func(r Repo) error {
		if err := r.InsertTx(ctx, t); err != nil {
			return err
		}
		if consumeOdo {
			return r.ConsumeOdometer(ctx, t.DriverID, id)
		}
		return nil
	})
	if err != nil {
		if rerr := s.payments.Refund(ctx, id); rerr != nil {
			slog.ErrorContext(ctx, "refund after failed record", "tx", id, "err", rerr)
		}
		return Transaction{}, err
	}
	return t.withLitres(), nil
}

// ---------- station: scan and dispense ----------

// ScanInput identifies a vehicle at the pump.
type ScanInput struct {
	Method    Source `json:"method"`    // qr | rfid
	Token     string `json:"token"`     // qr: the driver's pass token
	VehicleID string `json:"vehicleId"` // rfid: the tag's vehicle (simulated reader)
}

// ScanResult is what the attendant sees after identifying a vehicle.
type ScanResult struct {
	ScanID     string       `json:"scanId"`
	Source     Source       `json:"source"`
	Token      string       `json:"token,omitempty"`
	Vehicle    FleetVehicle `json:"vehicle"`
	OrgName    i18n.Text    `json:"orgName"`
	Driver     *FleetDriver `json:"driver"`
	Available  int64        `json:"available"`
	Reason     string       `json:"reason"`
	FuelLocked bool         `json:"fuelLocked"` // only the vehicle's grade may be dispensed
	Active     bool         `json:"active"`
	Block      *FleetBlock  `json:"block"`
}

// Scan identifies a vehicle by QR pass or RFID tag at the caller's station.
// Suspended drivers/vehicles are still identified (so the attendant sees why
// dispensing is blocked) and the attempt is written to the security log.
func (s *Service) Scan(ctx context.Context, c *authn.Claims, in ScanInput) (ScanResult, error) {
	var (
		vehicleID, token string
		claim            passClaim
	)
	switch in.Method {
	case SourceQR:
		found, err := s.kv.GetJSON(ctx, "pass:"+in.Token, &claim)
		if err != nil {
			return ScanResult{}, err
		}
		if !found || in.Token == "" {
			return ScanResult{}, errPassExpired
		}
		vehicleID, token = claim.VehicleID, in.Token
	case SourceRFID:
		if in.VehicleID == "" {
			return ScanResult{}, errMethod
		}
		vehicleID = in.VehicleID
	default:
		return ScanResult{}, errMethod
	}
	fc, err := s.fleet.VehicleContext(ctx, vehicleID)
	if err != nil {
		return ScanResult{}, err
	}
	if fc.Vehicle == nil {
		return ScanResult{}, errNoVehicle
	}
	// The pass is bound to a driver/vehicle pair; a reassignment invalidates it.
	if in.Method == SourceQR && (fc.Driver == nil || fc.Driver.ID != claim.DriverID) {
		return ScanResult{}, errPassExpired
	}
	active := fc.Block == nil
	if in.Method == SourceQR && active && fc.Rules.Odometer {
		odo, err := s.store.PendingOdometer(ctx, fc.Driver.ID)
		if err != nil {
			return ScanResult{}, err
		}
		if odo == nil {
			return ScanResult{}, errPassInactive
		}
	}
	if !InNetwork(fc.Rules, c.Station) {
		return ScanResult{}, errNotInNetwork
	}
	if !active {
		s.logBlockedAttempt(ctx, fc, c.Station)
	}
	res := ScanResult{Source: in.Method, Token: token, Vehicle: *fc.Vehicle, OrgName: fc.OrgName, Driver: fc.Driver,
		FuelLocked: fc.Rules.WrongFuel, Active: active, Block: fc.Block, Reason: ReasonLimit}
	if al, err := s.allowanceView(ctx, fc); err != nil {
		return ScanResult{}, err
	} else if al != nil {
		res.Available, res.Reason = al.Available, al.Reason
	}
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	res.ScanID = hex.EncodeToString(b)
	sess := scanSession{ID: res.ScanID, Source: in.Method, Token: token, VehicleID: fc.Vehicle.ID, StationID: c.Station}
	if _, err := s.kv.SetJSON(ctx, "scan:"+res.ScanID, sess, s.scanTTL, false); err != nil {
		return ScanResult{}, err
	}
	return res, nil
}

// logBlockedAttempt writes a security event for a scan of a suspended driver
// or vehicle (prototype logSec). Failures are logged, not returned: the scan
// result must still reach the attendant.
func (s *Service) logBlockedAttempt(ctx context.Context, fc FleetContext, stationID string) {
	stn := i18n.T(stationID, stationID)
	if info, err := s.stations.Station(ctx, stationID); err == nil {
		stn = info.Name
	}
	ev := SecurityEvent{OrgID: fc.OrgID, Severity: SeverityCritical, CreatedAt: s.now()}
	if fc.Block.Kind == "driver" && fc.Driver != nil {
		d := fc.Driver
		ev.Title = i18n.T("محاولة تعبئة من سائق موقوف", "Fueling attempt by a suspended driver")
		ev.Detail = i18n.T("حاول "+d.Name.Ar+" التعبئة في "+stn.Ar+" أثناء إيقاف حسابه، فمُنعت العملية.",
			d.Name.En+" attempted to fuel at "+stn.En+" while suspended. The attempt was blocked.")
		ev.Ref = d.Code
	} else {
		p := fc.Vehicle.Plate
		ev.Title = i18n.T("محاولة تعبئة لمركبة موقوفة", "Fueling attempt on a suspended vehicle")
		ev.Detail = i18n.T("مُسح رمز المركبة الموقوفة "+p.Digits+" في "+stn.Ar+"، فمُنعت العملية.",
			"The code for suspended vehicle "+p.String()+" was scanned at "+stn.En+". The attempt was blocked.")
		ev.Ref = p.Digits + "-" + p.Latin
	}
	if err := s.store.InsertSecurityEvent(ctx, ev); err != nil {
		slog.ErrorContext(ctx, "security event not recorded", "err", err)
	}
}

// DispenseInput starts a pump for a scanned vehicle. Give either Litres or
// Amount (SDG); litres are converted at the pump's current price.
type DispenseInput struct {
	ScanID string  `json:"scanId"`
	Pump   int     `json:"pump"`
	Amount int64   `json:"amount"`
	Litres float64 `json:"litres"`
}

// Dispense charges the company account and creates a "fueling" transaction
// for a previously scanned vehicle. Everything checked at scan time is
// re-checked, since state may have changed in between.
func (s *Service) Dispense(ctx context.Context, c *authn.Claims, in DispenseInput) (Transaction, error) {
	var sess scanSession
	found, err := s.kv.GetJSON(ctx, "scan:"+in.ScanID, &sess)
	if err != nil {
		return Transaction{}, err
	}
	if !found || in.ScanID == "" || sess.StationID != c.Station {
		return Transaction{}, errScanExpired
	}
	fc, err := s.fleet.VehicleContext(ctx, sess.VehicleID)
	if err != nil {
		return Transaction{}, err
	}
	if fc.Block != nil {
		if fc.Block.Kind == "driver" {
			return Transaction{}, errDriverBlock
		}
		return Transaction{}, errVehBlock
	}
	var odo *OdometerReading
	if fc.Driver != nil {
		if odo, err = s.store.PendingOdometer(ctx, fc.Driver.ID); err != nil {
			return Transaction{}, err
		}
	}
	if sess.Source == SourceQR && fc.Rules.Odometer && odo == nil {
		return Transaction{}, errPassInactive
	}
	if !InNetwork(fc.Rules, c.Station) {
		return Transaction{}, errNotInNetwork
	}
	fuel := fc.Vehicle.Fuel
	q, err := s.stations.Quote(ctx, c.Station, in.Pump)
	if err != nil {
		return Transaction{}, err
	}
	if !q.Pump.Online || !slices.Contains(q.Pump.Grades, fuel) {
		return Transaction{}, errPumpOffline
	}
	if busy, err := s.store.PumpBusy(ctx, c.Station, in.Pump); err != nil {
		return Transaction{}, err
	} else if busy {
		return Transaction{}, errPumpBusy
	}
	price := q.Prices[fuel]
	amount := in.Amount
	if in.Litres > 0 {
		amount = int64(math.Round(in.Litres * float64(price)))
	}
	if amount <= 0 || amount > common.MaxMoney {
		return Transaction{}, errAmount
	}
	al, err := s.payments.Allowance(ctx, fc.OrgID, fc.Vehicle.ID)
	if err != nil {
		return Transaction{}, err
	}
	if cp, reason := Cap(fc.Vehicle.Limit, al.Used, al.Balance, fc.Rules); amount > cp {
		return Transaction{}, capError(cp, reason)
	}
	now := s.now()
	t := Transaction{
		OrgID: fc.OrgID, VehicleID: fc.Vehicle.ID, Plate: fc.Vehicle.Plate, StationID: c.Station, Pump: in.Pump,
		Fuel: fuel, Amount: amount, PricePerLitre: price, Status: StatusFueling, Source: sess.Source, CreatedAt: now, StartedAt: &now,
	}
	if fc.Driver != nil {
		t.DriverID = fc.Driver.ID
	}
	consume := sess.Source == SourceQR && odo != nil
	if consume {
		t.OdometerKm = &odo.Km
	}
	out, err := s.chargeAndRecord(ctx, t, fc.Vehicle.Limit, consume)
	if err != nil {
		return Transaction{}, err
	}
	_ = s.kv.Delete(ctx, "scan:"+in.ScanID)
	return out, nil
}

// ---------- station: lifecycle ----------

// transition moves a transaction of the caller's station from one status to
// the next. Completing a fill-up enqueues the completed event atomically.
func (s *Service) transition(ctx context.Context, c *authn.Claims, find func(Repo) (Transaction, error), from, to Status) (Transaction, error) {
	var out Transaction
	err := s.store.InTx(ctx, func(r Repo) error {
		t, err := find(r)
		if errors.Is(err, ErrNotFound) || (err == nil && t.StationID != c.Station) {
			return errNotFound
		} else if err != nil {
			return err
		}
		if t.Status != from {
			return errState
		}
		now := s.now()
		t.Status = to
		if to == StatusFueling {
			t.StartedAt = &now
		}
		if to == StatusDone {
			t.CompletedAt = &now
			ev, err := events.New("fueling", events.TypeTransactionCompleted, events.TransactionCompleted{
				TransactionID: t.ID, StationID: t.StationID, OrgID: t.OrgID, Amount: t.Amount, CompletedAt: now,
			})
			if err != nil {
				return err
			}
			if err := r.Enqueue(ctx, ev); err != nil {
				return err
			}
		}
		out = t
		return r.UpdateTx(ctx, t)
	})
	return out.withLitres(), err
}

// Start turns on the pump for an app authorization.
func (s *Service) Start(ctx context.Context, c *authn.Claims, id string) (Transaction, error) {
	return s.transition(ctx, c, func(r Repo) (Transaction, error) { return r.GetTx(ctx, id) }, StatusAuthorized, StatusFueling)
}

// StartByCode starts the open authorization matching the driver's 4-digit code.
func (s *Service) StartByCode(ctx context.Context, c *authn.Claims, code string) (Transaction, error) {
	t, err := s.transition(ctx, c, func(r Repo) (Transaction, error) {
		t, err := r.FindOpenByCode(ctx, c.Station, code)
		if errors.Is(err, ErrNotFound) {
			return t, errCodeNotFound
		}
		return t, err
	}, StatusAuthorized, StatusFueling)
	return t, err
}

// Finish completes a running fill-up and issues the receipt.
func (s *Service) Finish(ctx context.Context, c *authn.Claims, id string) (Transaction, error) {
	return s.transition(ctx, c, func(r Repo) (Transaction, error) { return r.GetTx(ctx, id) }, StatusFueling, StatusDone)
}

// ---------- reads ----------

// Transactions lists the transactions visible to the caller: a driver's own,
// a fleet manager's org, or a station's.
func (s *Service) Transactions(ctx context.Context, c *authn.Claims) ([]Transaction, error) {
	var f TxFilter
	switch {
	case c.Role == authn.RoleDriver:
		f.DriverID = c.Actor
	case c.Role == authn.RoleFleetManager:
		f.OrgID = c.Org
	case c.Role.IsStationStaff():
		f.StationID = c.Station
	default:
		return nil, httpx.ErrForbidden
	}
	ts, err := s.store.ListTx(ctx, f, 200)
	for i := range ts {
		ts[i] = ts[i].withLitres()
	}
	return ts, err
}

// SecurityEvents lists the org's security log, newest first.
func (s *Service) SecurityEvents(ctx context.Context, orgID string) ([]SecurityEvent, error) {
	return s.store.ListSecurityEvents(ctx, orgID, 100)
}
