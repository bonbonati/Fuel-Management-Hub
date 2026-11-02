package fueling

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/bonbonati/fuel-management-hub/backend/internal/common"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/i18n"
)

// This file holds the fueling service's pure rules (prototype: capFor,
// odoLocked, the pay sheet and station scanner validations). Covered by
// rules_test.go.

// Cap reasons.
const (
	ReasonLimit   = "limit"   // bounded by remaining monthly limit or wallet balance
	ReasonPerFill = "perfill" // bounded by the org's max-per-fill-up rule
)

// Cap is how much a vehicle may fuel right now: the smaller of its remaining
// monthly limit and the org's wallet balance (never negative), further capped
// by the max-per-fill-up rule when enabled.
func Cap(limit, used, balance int64, r FleetRules) (int64, string) {
	c := max(0, min(limit-used, balance))
	if r.MaxFill && r.MaxFillAmount < c {
		return r.MaxFillAmount, ReasonPerFill
	}
	return c, ReasonLimit
}

// InNetwork reports whether stationID is allowed by the network rule.
func InNetwork(r FleetRules, stationID string) bool {
	return !r.Network || slices.Contains(r.NetworkStationIDs, stationID)
}

// odometerPattern matches a plausible odometer reading (3–7 digits).
var odometerPattern = regexp.MustCompile(`^\d{3,7}$`)

// ParseOdometer validates an odometer reading typed by the driver.
func ParseOdometer(s string) (int, error) {
	s = strings.TrimSpace(s)
	if !odometerPattern.MatchString(s) {
		return 0, errOdometerFormat
	}
	n, _ := strconv.Atoi(s)
	return n, nil
}

// NewPassToken builds a rotating pass token: PF-<driver digits>-<grade>-<random>,
// e.g. "PF-3071-95-7C1A9F02". The random part (32 bits) makes tokens
// unguessable within their 30-second lifetime.
func NewPassToken(driverCode string, fuel common.Fuel) string {
	digits := strings.TrimPrefix(strings.TrimPrefix(driverCode, "D-"), "d-")
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("PF-%s-%s-%s", digits, fuel.PassTag(), strings.ToUpper(hex.EncodeToString(b)))
}

// StartCode returns a random 4-digit code (1000–9999) shown in the driver's
// app and typed by the attendant to start the pump.
func StartCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(9000))
	return strconv.FormatInt(1000+n.Int64(), 10)
}

// FormatSDG renders an amount like the apps do: "200,000 ج.س" / "200,000 SDG".
func FormatSDG(n int64) i18n.Text {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	out := b.String()
	if neg {
		out = "-" + out
	}
	return i18n.T(out+" ج.س", out+" SDG")
}

// capError explains why an amount exceeds the cap (prototype capMsg).
func capError(c int64, reason string) error {
	m := FormatSDG(c)
	if reason == ReasonPerFill {
		return httpx.Conflict("exceeds_per_fill",
			"الحد الأقصى للتعبئة الواحدة وفق سياسة الشركة: "+m.Ar, "Company policy caps a single fill-up at "+m.En)
	}
	return httpx.Conflict("exceeds_available", "المبلغ يتجاوز المتاح لك ("+m.Ar+")", "The amount exceeds your available "+m.En)
}

// Errors returned by the fueling service.
var (
	errNotFound       = httpx.NotFound("not_found", "العملية غير موجودة.", "Transaction not found.")
	errOdometerFormat = httpx.BadRequest("invalid_odometer", "أدخل قراءة عدّاد صحيحة بالأرقام", "Enter a valid odometer reading in digits")
	errOdometerNeeded = httpx.Conflict("odometer_required", "سجّل قراءة العدّاد أولاً", "Record the odometer reading first")
	errPassInactive   = httpx.Conflict("pass_inactive",
		"تصريح السائق غير مفعّل: لم تُسجَّل قراءة العدّاد بعد.", "Driver pass inactive: odometer not recorded yet.")
	errNotInNetwork = httpx.Conflict("station_not_in_network", "المحطة خارج شبكة الشركة", "This station is not in the company network")
	errWrongFuel    = httpx.Conflict("wrong_fuel",
		"نوع الوقود غير مسموح لهذه المركبة.", "This fuel grade is not allowed for this vehicle.")
	errBadFuel     = httpx.BadRequest("invalid_fuel", "نوع الوقود غير معروف.", "Unknown fuel grade.")
	errPumpOffline = httpx.Conflict("pump_unavailable",
		"المضخة متوقفة أو لا توفر هذا النوع من الوقود.", "The pump is offline or does not dispense this grade.")
	errPumpBusy  = httpx.Conflict("pump_busy", "المضخة قيد الاستخدام.", "This pump is in use.")
	errMinAmount = httpx.BadRequest("invalid_amount",
		"الحد الأدنى للتعبئة 10,000 ج.س", "The minimum fill-up is SDG 10,000")
	errAmount      = httpx.BadRequest("invalid_amount", "الكمية غير صالحة.", "The quantity is not valid.")
	errNoVehicle   = httpx.Conflict("no_vehicle", "لا توجد مركبة مخصصة لك.", "No vehicle is assigned to you.")
	errPassExpired = httpx.NotFound("pass_expired",
		"انتهت صلاحية رمز التصريح. اطلب من السائق تحديثه.", "The pass code has expired. Ask the driver to refresh it.")
	errScanExpired = httpx.NotFound("scan_expired",
		"انتهت جلسة المسح. امسح المركبة مرة أخرى.", "The scan session has expired. Scan the vehicle again.")
	errCodeNotFound = httpx.NotFound("code_not_found",
		"لا توجد عملية مفتوحة بهذا الرمز. اطلب من السائق التحقق من التطبيق.",
		"No open authorization matches this code. Ask the driver to check the app.")
	errState = httpx.Conflict("invalid_state",
		"لا يمكن تنفيذ هذا الإجراء على العملية في حالتها الحالية.", "This action is not possible in the transaction's current state.")
	errMethod      = httpx.BadRequest("invalid_method", "طريقة التعرّف غير معروفة.", "Unknown identification method.")
	errDriverBlock = httpx.Conflict("blocked", "حساب السائق موقوف. لا يمكن الصرف.", "The driver account is suspended. Dispensing is blocked.")
	errVehBlock    = httpx.Conflict("blocked", "المركبة موقوفة. لا يمكن الصرف.", "The vehicle is suspended. Dispensing is blocked.")
)

// blockError converts a fleet block into an API error with its message.
func blockError(b *FleetBlock) error {
	return &httpx.Error{Status: 409, Code: "blocked", Message: b.Message}
}
