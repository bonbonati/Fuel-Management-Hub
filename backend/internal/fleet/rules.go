package fleet

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/bonbonati/fuel-management-hub/backend/internal/common"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/i18n"
)

// This file holds the fleet's pure business rules (no I/O), ported from the
// prototype's limOf/baseLim, driverBlock, openReq/sendReq and modal
// validations. Every rule here is covered by rules_test.go.

// BaseLimit is the vehicle's monthly limit before approved increases: its
// custom limit when set, otherwise its tier's per-vehicle budget, otherwise 0.
func BaseLimit(v Vehicle, t *Tier) int64 {
	if v.CustomLimit != nil {
		return *v.CustomLimit
	}
	if t != nil {
		return t.Budget
	}
	return 0
}

// Limit is the vehicle's effective monthly limit: BaseLimit + Extra.
func Limit(v Vehicle, t *Tier) int64 { return BaseLimit(v, t) + v.Extra }

// View attaches computed limits to v.
func View(v Vehicle, t *Tier) VehicleView {
	return VehicleView{Vehicle: v, BaseLimit: BaseLimit(v, t), Limit: Limit(v, t)}
}

// ComputeBlock reports why fueling is blocked (driver suspended, no vehicle,
// vehicle suspended), or nil when allowed. Checks run in that order, matching
// the prototype. d may be nil for a vehicle without a driver.
func ComputeBlock(d *Driver, v *Vehicle) *Block {
	switch {
	case d != nil && !d.Active:
		return &Block{Kind: BlockDriver, Message: i18n.T(
			"حسابك موقوف من قِبل مدير الأسطول. يمكنك تقديم طلب إعادة تفعيل.",
			"Your account has been suspended by your fleet manager. You can submit a reactivation request.")}
	case v == nil:
		return &Block{Kind: BlockNoVehicle, Message: i18n.T("لا توجد مركبة مخصصة لك حالياً.", "No vehicle is assigned to you.")}
	case !v.Active:
		return &Block{Kind: BlockVehicle, Message: i18n.T(
			"المركبة المخصصة لك موقوفة من قِبل مدير الأسطول. يمكنك تقديم طلب إعادة تفعيل.",
			"Your assigned vehicle has been suspended by your fleet manager. You can submit a reactivation request.")}
	}
	return nil
}

// sudanPhone matches a normalized Sudanese mobile number in digits only.
var sudanPhone = regexp.MustCompile(`^249\d{9}$`)

// NormalizePhone validates a Sudanese mobile number in any spacing and
// returns it as "+249 XX XXX XXXX".
func NormalizePhone(in string) (string, error) {
	var b strings.Builder
	for _, r := range in {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	if !sudanPhone.MatchString(d) {
		return "", errPhone
	}
	return "+249 " + d[3:5] + " " + d[5:8] + " " + d[8:], nil
}

// Reasons allowed per request type (matching the request sheet options).
var requestReasons = map[RequestType][]string{
	RequestLimit:    {"trip", "extra", "emergency", "other"},
	RequestActivate: {"review", "emergency", "other"},
}

// maxNoteRunes caps request notes (textarea maxlength in the app).
const maxNoteRunes = 280

// RequestInput is a driver's new request.
type RequestInput struct {
	Type   RequestType `json:"type"`
	Amount int64       `json:"amount"`
	Reason string      `json:"reason"`
	Note   string      `json:"note"`
}

// ValidateRequest checks a new driver request against the driver's current
// block state and their pending requests:
//   - a limit increase is not available while blocked;
//   - reactivation is only available when the driver or vehicle is suspended
//     (not merely when no vehicle is assigned);
//   - only one pending request per type;
//   - limit amounts must be within [MinFillAmount, MaxRequestAmount].
func ValidateRequest(in RequestInput, block *Block, pending []Request) error {
	canActivate := block != nil && block.Kind != BlockNoVehicle
	switch in.Type {
	case RequestLimit:
		if block != nil {
			return errLimitWhileBlocked
		}
		if in.Amount < common.MinFillAmount || in.Amount > common.MaxRequestAmount {
			return errRequestAmount
		}
	case RequestActivate:
		if !canActivate {
			return errNothingToActivate
		}
	default:
		return errRequestType
	}
	if !slices.Contains(requestReasons[in.Type], in.Reason) {
		return errRequestReason
	}
	if utf8.RuneCountInString(in.Note) > maxNoteRunes {
		return errNoteTooLong
	}
	for _, p := range pending {
		if p.Status == StatusPending && p.Type == in.Type {
			return errDuplicateRequest
		}
	}
	return nil
}

// Errors returned by the fleet service. Messages reuse the prototype's copy.
var (
	errNotFound = httpx.NotFound("not_found", "العنصر غير موجود.", "Not found.")

	errPhone = httpx.BadRequest("invalid_phone",
		"يجب أن يبدأ رقم الجوال بـ +249 ويليه 9 أرقام.", "The mobile number must be +249 followed by 9 digits.")
	errRequestAmount = httpx.BadRequest("invalid_amount",
		"أدخل مبلغاً بين 10,000 و 2,000,000 ج.س", "Enter an amount between 10,000 and 2,000,000 SDG")
	errLimitWhileBlocked = httpx.Conflict("limit_unavailable",
		"طلب زيادة الحد غير متاح أثناء الإيقاف.", "A limit increase is unavailable while suspended.")
	errNothingToActivate = httpx.Conflict("nothing_to_activate",
		"حسابك ومركبتك مفعّلان.", "Your account and vehicle are already active.")
	errRequestType   = httpx.BadRequest("invalid_type", "نوع الطلب غير معروف.", "Unknown request type.")
	errRequestReason = httpx.BadRequest("invalid_reason", "سبب الطلب غير صالح.", "Invalid request reason.")
	errNoteTooLong   = httpx.BadRequest("note_too_long",
		"التفاصيل أطول من 280 حرفاً.", "The details are longer than 280 characters.")
	errDuplicateRequest = httpx.Conflict("duplicate_request",
		"لديك طلب من هذا النوع قيد المراجعة. انتظر القرار قبل تقديم طلب جديد.",
		"You already have a request of this type under review. Please wait for a decision.")
	errNotPending = httpx.Conflict("not_pending",
		"تم البت في هذا الطلب مسبقاً.", "This request has already been decided.")
	errApprovedAmount = httpx.BadRequest("invalid_amount",
		"أدخل مبلغاً معتمداً لا يقل عن 10,000 ج.س", "Enter an approved amount of at least SDG 10,000")
	errNoVehicleOnRequest = httpx.Conflict("no_vehicle",
		"لا توجد مركبة مرتبطة بهذا الطلب", "No vehicle is linked to this request")
	errAmount = httpx.BadRequest("invalid_amount",
		"المبلغ غير صالح.", "The amount is not valid.")
	errMaxFill = httpx.BadRequest("invalid_amount",
		"الحد الأقصى للتعبئة الواحدة يجب ألا يقل عن 10,000 ج.س", "The maximum per fill-up must be at least SDG 10,000")
	errDuplicateCode = httpx.Conflict("duplicate_code",
		"رقم السائق مستخدم لسائق آخر.", "This driver ID is already used by another driver.")
)

// fieldErr builds a 400 for a missing/invalid form field, naming the field
// in both languages ("Please check the "X" field" in the prototype).
func fieldErr(ar, en string) error {
	return httpx.BadRequest("invalid_field", "يرجى مراجعة حقل «"+ar+"»", `Please check the "`+en+`" field`)
}
