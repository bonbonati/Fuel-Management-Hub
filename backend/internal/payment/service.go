// Package payment is the payment service: each fleet org's prepaid wallet
// and its ledger, monthly per-vehicle usage, VAT settings, and station
// settlements (payouts).
//
// Money invariants live here. A charge is checked and recorded atomically
// while holding a row lock on the org's wallet, so concurrent fill-ups can
// never overdraw the wallet or push a vehicle past its monthly limit. Charges
// and refunds are idempotent by transaction id, so the fueling service can
// safely retry.
//
// Settlements are accrued asynchronously from "fueling.transaction.completed"
// events (see consumer.go); a station's payout never blocks a pump.
package payment

import (
	"context"
	"errors"
	"time"
	_ "time/tzdata" // embed tz database: distroless images have none

	"github.com/bonbonati/fuel-management-hub/backend/internal/common"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
)

// Ledger entry kinds.
const (
	KindOpening = "opening_usage" // usage carried in when an org is onboarded mid-month
	KindCharge  = "charge"        // fill-up debit
	KindTopUp   = "topup"         // prepaid balance credit
	KindRefund  = "refund"        // reversal of a charge
)

// Entry is one ledger row. Amounts are positive; Kind gives the direction.
type Entry struct {
	OrgID     string
	VehicleID *string
	TxID      *string // fueling transaction id (charges/refunds), unique per kind
	Kind      string
	Amount    int64
	CreatedAt time.Time
}

// Wallet is an org's balance and this month's usage per vehicle.
type Wallet struct {
	OrgID   string           `json:"orgId"`
	Balance int64            `json:"balance"`
	Month   string           `json:"month"` // YYYY-MM (Africa/Khartoum)
	Usage   map[string]int64 `json:"usage"` // vehicle id → SDG used this month
}

// Tax is the VAT configuration. Pump prices are VAT inclusive.
type Tax struct {
	Enabled bool    `json:"enabled"`
	Rate    float64 `json:"rate"` // percent, 0–100
}

// Settlement is one daily payout to a station.
type Settlement struct {
	StationID string `json:"stationId"`
	Date      string `json:"date"` // YYYY-MM-DD
	Amount    int64  `json:"amount"`
	Fee       int64  `json:"fee"`    // platform fee withheld
	Status    string `json:"status"` // scheduled | paid
}

// ErrNotFound is returned by Repo getters for missing rows.
var ErrNotFound = errors.New("payment: not found")

// Repo is the persistence port. Inside Store.InTx, LockWallet holds the
// wallet row lock until commit.
type Repo interface {
	LockWallet(ctx context.Context, orgID string) (int64, error)
	SetBalance(ctx context.Context, orgID string, balance int64) error
	FindEntry(ctx context.Context, kind, txID string) (Entry, error)
	InsertEntry(ctx context.Context, e Entry) error
	VehicleUsage(ctx context.Context, orgID, vehicleID string, since time.Time) (int64, error)
	OrgUsage(ctx context.Context, orgID string, since time.Time) (map[string]int64, error)
	GetTax(ctx context.Context) (Tax, error)
	SetTax(ctx context.Context, t Tax) error
	ListSettlements(ctx context.Context, stationID string, limit int) ([]Settlement, error)
	AccrueSettlement(ctx context.Context, stationID, date string, amount, fee int64) error
	// MarkProcessed records an event id; it returns false if it was already processed.
	MarkProcessed(ctx context.Context, eventID string) (bool, error)
}

// Store is a Repo that can run a function in a transaction.
type Store interface {
	Repo
	InTx(ctx context.Context, fn func(Repo) error) error
}

// Service implements the payment use cases.
type Service struct {
	store  Store
	loc    *time.Location
	feeBps int64 // settlement platform fee in basis points
	now    func() time.Time
}

// NewService returns a Service. feeBps is the platform fee withheld from
// station settlements (e.g. 43 = 0.43%).
func NewService(store Store, feeBps int64) *Service {
	loc, err := time.LoadLocation("Africa/Khartoum")
	if err != nil {
		loc = time.UTC
	}
	return &Service{store: store, loc: loc, feeBps: feeBps, now: time.Now}
}

// monthStart is the first instant of the current month in Khartoum time;
// vehicle limits reset monthly.
func (s *Service) monthStart() time.Time {
	n := s.now().In(s.loc)
	return time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, s.loc)
}

var (
	errInsufficient = httpx.Conflict("insufficient_balance",
		"رصيد حساب الشركة غير كافٍ لهذه التعبئة.", "The company balance is not enough for this fill-up.")
	errLimit = httpx.Conflict("limit_exceeded",
		"المبلغ يتجاوز المتاح لهذه المركبة هذا الشهر.", "The amount exceeds this vehicle's available limit this month.")
	errAmount = httpx.BadRequest("invalid_amount", "المبلغ غير صالح.", "The amount is not valid.")
	errTopUp  = httpx.BadRequest("invalid_amount",
		"أدخل مبلغ شحن بين 10,000 و 1,000,000,000 ج.س", "Enter a top-up between SDG 10,000 and 1,000,000,000")
	errRate      = httpx.BadRequest("invalid_rate", "أدخل نسبة بين 0 و 100", "Enter a rate between 0 and 100")
	errNoWallet  = httpx.NotFound("no_wallet", "لا يوجد حساب مسبق الدفع لهذه الشركة.", "This company has no prepaid account.")
	errNoCharge  = httpx.NotFound("no_charge", "لا توجد عملية خصم بهذا الرقم.", "No charge exists for this transaction.")
	errForbidden = httpx.ErrForbidden
)

// CheckCharge is the pure money rule for a fill-up: the amount must be
// positive, fit the remaining monthly limit (limit - used), and be covered by
// the wallet balance. The limit is checked first so drivers see the more
// actionable message.
func CheckCharge(balance, used, limit, amount int64) error {
	switch {
	case amount <= 0 || amount > common.MaxMoney:
		return errAmount
	case used+amount > limit:
		return errLimit
	case amount > balance:
		return errInsufficient
	}
	return nil
}

// ChargeInput debits a fill-up from an org's wallet.
type ChargeInput struct {
	TransactionID string `json:"transactionId"`
	OrgID         string `json:"orgId"`
	VehicleID     string `json:"vehicleId"`
	Amount        int64  `json:"amount"`
	VehicleLimit  int64  `json:"vehicleLimit"` // effective monthly limit from the fleet service
}

// ChargeResult reports the state after a charge.
type ChargeResult struct {
	TransactionID string `json:"transactionId"`
	Balance       int64  `json:"balance"`
	Used          int64  `json:"used"`
	Replayed      bool   `json:"replayed"` // true when this charge was already recorded
}

// Charge atomically checks and records a fill-up debit. Replaying the same
// transaction id returns the original result without charging twice.
func (s *Service) Charge(ctx context.Context, in ChargeInput) (ChargeResult, error) {
	if in.TransactionID == "" || in.OrgID == "" || in.VehicleID == "" {
		return ChargeResult{}, errAmount.WithDetail("transactionId, orgId and vehicleId are required")
	}
	var out ChargeResult
	err := s.store.InTx(ctx, func(r Repo) error {
		balance, err := r.LockWallet(ctx, in.OrgID)
		if errors.Is(err, ErrNotFound) {
			return errNoWallet
		} else if err != nil {
			return err
		}
		used, err := r.VehicleUsage(ctx, in.OrgID, in.VehicleID, s.monthStart())
		if err != nil {
			return err
		}
		if _, err := r.FindEntry(ctx, KindCharge, in.TransactionID); err == nil {
			out = ChargeResult{TransactionID: in.TransactionID, Balance: balance, Used: used, Replayed: true}
			return nil
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		if err := CheckCharge(balance, used, in.VehicleLimit, in.Amount); err != nil {
			return err
		}
		tx, veh := in.TransactionID, in.VehicleID
		if err := r.InsertEntry(ctx, Entry{OrgID: in.OrgID, VehicleID: &veh, TxID: &tx, Kind: KindCharge, Amount: in.Amount, CreatedAt: s.now()}); err != nil {
			return err
		}
		if err := r.SetBalance(ctx, in.OrgID, balance-in.Amount); err != nil {
			return err
		}
		out = ChargeResult{TransactionID: in.TransactionID, Balance: balance - in.Amount, Used: used + in.Amount}
		return nil
	})
	return out, err
}

// Refund reverses a recorded charge (compensation when a fill-up could not
// be created after charging). Idempotent.
func (s *Service) Refund(ctx context.Context, txID string) error {
	return s.store.InTx(ctx, func(r Repo) error {
		charge, err := r.FindEntry(ctx, KindCharge, txID)
		if errors.Is(err, ErrNotFound) {
			return errNoCharge
		} else if err != nil {
			return err
		}
		if _, err := r.FindEntry(ctx, KindRefund, txID); err == nil {
			return nil // already refunded
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		balance, err := r.LockWallet(ctx, charge.OrgID)
		if err != nil {
			return err
		}
		if err := r.InsertEntry(ctx, Entry{OrgID: charge.OrgID, VehicleID: charge.VehicleID, TxID: &txID, Kind: KindRefund, Amount: charge.Amount, CreatedAt: s.now()}); err != nil {
			return err
		}
		return r.SetBalance(ctx, charge.OrgID, balance+charge.Amount)
	})
}

// Allowance is what the fueling service needs to compute a vehicle's cap.
type Allowance struct {
	Balance int64 `json:"balance"`
	Used    int64 `json:"used"`
}

// Allowance returns the org balance and the vehicle's usage this month.
func (s *Service) Allowance(ctx context.Context, orgID, vehicleID string) (Allowance, error) {
	balance, err := s.store.LockWallet(ctx, orgID) // outside a tx this is a plain read
	if errors.Is(err, ErrNotFound) {
		return Allowance{}, errNoWallet
	} else if err != nil {
		return Allowance{}, err
	}
	used, err := s.store.VehicleUsage(ctx, orgID, vehicleID, s.monthStart())
	return Allowance{Balance: balance, Used: used}, err
}

// Wallet returns the org's balance and per-vehicle usage this month.
func (s *Service) Wallet(ctx context.Context, orgID string) (Wallet, error) {
	balance, err := s.store.LockWallet(ctx, orgID)
	if errors.Is(err, ErrNotFound) {
		return Wallet{}, errNoWallet
	} else if err != nil {
		return Wallet{}, err
	}
	start := s.monthStart()
	usage, err := s.store.OrgUsage(ctx, orgID, start)
	if err != nil {
		return Wallet{}, err
	}
	return Wallet{OrgID: orgID, Balance: balance, Month: start.Format("2006-01"), Usage: usage}, nil
}

// TopUp credits the org's prepaid balance.
func (s *Service) TopUp(ctx context.Context, orgID string, amount int64) (Wallet, error) {
	if amount < common.MinFillAmount || amount > common.MaxMoney {
		return Wallet{}, errTopUp
	}
	err := s.store.InTx(ctx, func(r Repo) error {
		balance, err := r.LockWallet(ctx, orgID)
		if errors.Is(err, ErrNotFound) {
			return errNoWallet
		} else if err != nil {
			return err
		}
		if err := r.InsertEntry(ctx, Entry{OrgID: orgID, Kind: KindTopUp, Amount: amount, CreatedAt: s.now()}); err != nil {
			return err
		}
		return r.SetBalance(ctx, orgID, balance+amount)
	})
	if err != nil {
		return Wallet{}, err
	}
	return s.Wallet(ctx, orgID)
}

// Tax returns the VAT settings.
func (s *Service) Tax(ctx context.Context) (Tax, error) { return s.store.GetTax(ctx) }

// SetTax updates the VAT settings (rate 0–100%).
func (s *Service) SetTax(ctx context.Context, t Tax) (Tax, error) {
	if t.Rate < 0 || t.Rate > 100 {
		return Tax{}, errRate
	}
	return t, s.store.SetTax(ctx, t)
}

// SettlementSummary is the station's payout overview.
type SettlementSummary struct {
	StationID     string       `json:"stationId"`
	NextPayout    *Settlement  `json:"nextPayout"`
	FeesThisMonth int64        `json:"feesThisMonth"`
	Items         []Settlement `json:"items"` // newest first
}

// Settlements returns the station's recent payouts.
func (s *Service) Settlements(ctx context.Context, stationID string) (SettlementSummary, error) {
	items, err := s.store.ListSettlements(ctx, stationID, 30)
	if err != nil {
		return SettlementSummary{}, err
	}
	out := SettlementSummary{StationID: stationID, Items: items}
	month := s.now().In(s.loc).Format("2006-01")
	for i := range items {
		if len(items[i].Date) >= 7 && items[i].Date[:7] == month {
			out.FeesThisMonth += items[i].Fee
		}
		// Items are newest first, so the last scheduled one seen is the earliest.
		if items[i].Status == "scheduled" {
			out.NextPayout = &items[i]
		}
	}
	return out, nil
}

// SettlementFee is the platform fee withheld from amount.
func (s *Service) SettlementFee(amount int64) int64 { return amount * s.feeBps / 10_000 }

// AccrueCompleted adds a completed fill-up to the station's next payout
// (the day after completion, Khartoum time). It is idempotent per event id.
func (s *Service) AccrueCompleted(ctx context.Context, eventID, stationID string, amount int64, completedAt time.Time) error {
	if stationID == "" || amount <= 0 {
		return nil // nothing to settle
	}
	return s.store.InTx(ctx, func(r Repo) error {
		fresh, err := r.MarkProcessed(ctx, eventID)
		if err != nil || !fresh {
			return err
		}
		date := completedAt.In(s.loc).AddDate(0, 0, 1).Format("2006-01-02")
		return r.AccrueSettlement(ctx, stationID, date, amount, s.SettlementFee(amount))
	})
}
