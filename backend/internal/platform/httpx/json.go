package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
)

// maxBodyBytes caps request bodies; no endpoint accepts more than a small form.
const maxBodyBytes = 1 << 20

// HandlerFunc is an http.HandlerFunc that may return an error. Returned errors
// are rendered by WriteError, which keeps handlers free of error plumbing.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Handle adapts a HandlerFunc into a standard http.HandlerFunc.
func Handle(h HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			WriteError(w, r, err)
		}
	}
}

// WriteJSON writes v as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return nil
	}
	return json.NewEncoder(w).Encode(v)
}

// OK writes v as a 200 JSON response.
func OK(w http.ResponseWriter, v any) error { return WriteJSON(w, http.StatusOK, v) }

// Created writes v as a 201 JSON response.
func Created(w http.ResponseWriter, v any) error { return WriteJSON(w, http.StatusCreated, v) }

// WriteError renders err as a problem+json document. Unknown errors become a
// generic 500 and are logged with the request context; *Error values are
// rendered as-is (5xx ones are logged too).
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	e := AsError(err)
	if e == nil {
		slog.ErrorContext(r.Context(), "unhandled error", "err", err, "method", r.Method, "path", r.URL.Path)
		e = ErrInternal
	} else if e.Status >= 500 {
		slog.ErrorContext(r.Context(), "server error", "err", err, "method", r.Method, "path", r.URL.Path)
	}
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(toProblem(e))
}

// DecodeJSON decodes the request body into dst. Unknown fields are rejected so
// client typos surface as 400s instead of silently ignored input.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return ErrInvalidJSON.WithDetail("empty body")
		}
		return ErrInvalidJSON.WithDetail("%v", err)
	}
	return nil
}
