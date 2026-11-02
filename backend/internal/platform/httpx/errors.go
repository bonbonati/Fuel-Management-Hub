// Package httpx is the small HTTP toolkit shared by every service: typed API
// errors rendered as RFC 9457 problem documents, JSON helpers, middleware, a
// graceful server and health endpoints. It deliberately builds on net/http
// (Go 1.22+ pattern routing) instead of a third-party router.
package httpx

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/i18n"
)

// Error is an API error that is safe to show to end users.
//
// Code is a stable machine-readable identifier (snake_case) that clients may
// switch on; Message is the bilingual human text the apps display in a toast.
// Any error that is not an *Error is reported as a generic 500 so internal
// details never leak to clients.
type Error struct {
	Status  int       // HTTP status code
	Code    string    // stable machine code, e.g. "limit_exceeded"
	Message i18n.Text // bilingual, user-facing explanation
	Detail  string    // optional developer detail (English), safe to expose
}

func (e *Error) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%d %s: %s (%s)", e.Status, e.Code, e.Message.En, e.Detail)
	}
	return fmt.Sprintf("%d %s: %s", e.Status, e.Code, e.Message.En)
}

// NewError builds an *Error with the given status, code and bilingual message.
func NewError(status int, code, ar, en string) *Error {
	return &Error{Status: status, Code: code, Message: i18n.T(ar, en)}
}

// WithDetail returns a copy of e carrying a developer-facing detail string.
func (e *Error) WithDetail(format string, args ...any) *Error {
	c := *e
	c.Detail = fmt.Sprintf(format, args...)
	return &c
}

// Convenience constructors for the most common statuses.

func BadRequest(code, ar, en string) *Error { return NewError(http.StatusBadRequest, code, ar, en) }
func NotFound(code, ar, en string) *Error   { return NewError(http.StatusNotFound, code, ar, en) }
func Conflict(code, ar, en string) *Error   { return NewError(http.StatusConflict, code, ar, en) }
func Forbidden(code, ar, en string) *Error  { return NewError(http.StatusForbidden, code, ar, en) }

// Shared errors used across services.
var (
	ErrUnauthorized = NewError(http.StatusUnauthorized, "unauthorized",
		"يلزم تسجيل الدخول.", "Sign-in is required.")
	ErrForbidden = NewError(http.StatusForbidden, "forbidden",
		"ليست لديك صلاحية لهذا الإجراء.", "You do not have permission for this action.")
	ErrInvalidJSON = NewError(http.StatusBadRequest, "invalid_json",
		"تعذّرت قراءة الطلب.", "The request body could not be read.")
	ErrInternal = NewError(http.StatusInternalServerError, "internal",
		"حدث خطأ غير متوقع. حاول مرة أخرى.", "Something went wrong. Please try again.")
	ErrUnavailable = NewError(http.StatusServiceUnavailable, "unavailable",
		"الخدمة غير متاحة مؤقتاً. حاول بعد قليل.", "The service is temporarily unavailable. Please try again shortly.")
)

// AsError extracts an *Error from err (following wrapping), or returns nil.
func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}

// problem is the RFC 9457 wire format, extended with code and bilingual message.
type problem struct {
	Type    string    `json:"type"`
	Title   string    `json:"title"`
	Status  int       `json:"status"`
	Code    string    `json:"code"`
	Message i18n.Text `json:"message"`
	Detail  string    `json:"detail,omitempty"`
}

func toProblem(e *Error) problem {
	return problem{
		Type:    "about:blank",
		Title:   http.StatusText(e.Status),
		Status:  e.Status,
		Code:    e.Code,
		Message: e.Message,
		Detail:  e.Detail,
	}
}
