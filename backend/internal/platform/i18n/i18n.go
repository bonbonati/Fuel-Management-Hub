// Package i18n holds the bilingual text type shared by every service.
//
// PetroFleet is Arabic-first with English as the second language. Any value a
// person reads (names, addresses, error messages, security log entries) is
// stored and transported as a Text with both languages so the clients can
// switch language without another round trip.
package i18n

import "strings"

// Text is a bilingual string. JSON shape: {"ar": "...", "en": "..."}.
// It is persisted as JSONB in Postgres.
type Text struct {
	Ar string `json:"ar"`
	En string `json:"en"`
}

// T builds a Text, Arabic first (matching the frontend's T(ar, en) helper).
func T(ar, en string) Text { return Text{Ar: ar, En: en} }

// Same builds a Text with identical content in both languages. Used for
// free text typed by a user (request notes, replies) that is not translated.
func Same(s string) Text { return Text{Ar: s, En: s} }

// Trimmed returns a copy with surrounding whitespace removed from both languages.
func (t Text) Trimmed() Text {
	return Text{Ar: strings.TrimSpace(t.Ar), En: strings.TrimSpace(t.En)}
}

// Complete reports whether both languages are non-blank.
func (t Text) Complete() bool {
	return strings.TrimSpace(t.Ar) != "" && strings.TrimSpace(t.En) != ""
}

// IsZero reports whether both languages are blank.
func (t Text) IsZero() bool {
	return strings.TrimSpace(t.Ar) == "" && strings.TrimSpace(t.En) == ""
}
