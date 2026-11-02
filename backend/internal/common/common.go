// Package common holds the domain primitives shared by several services:
// fuel grades, Sudanese number plates and money bounds. It contains no I/O.
package common

import (
	"regexp"
	"slices"
	"strings"
)

// Fuel is a fuel grade. Keys match the prototype and the frontend:
// "91" regular benzene, "95" super benzene, "Diesel" gasoil.
type Fuel string

const (
	Fuel91     Fuel = "91"
	Fuel95     Fuel = "95"
	FuelDiesel Fuel = "Diesel"
)

// Fuels lists every grade in display order.
var Fuels = []Fuel{Fuel91, Fuel95, FuelDiesel}

// Valid reports whether f is a known grade.
func (f Fuel) Valid() bool { return slices.Contains(Fuels, f) }

// PassTag is the short grade code embedded in QR pass tokens ("DZ" for diesel).
func (f Fuel) PassTag() string {
	if f == FuelDiesel {
		return "DZ"
	}
	return string(f)
}

// DefaultPrices are the regulated pump prices in SDG per litre
// (Ministry of Energy and Oil), used to seed the station service.
var DefaultPrices = map[Fuel]int64{Fuel91: 3420, Fuel95: 3600, FuelDiesel: 3250}

// Money bounds, in whole Sudanese pounds (SDG). Amounts are int64 everywhere.
const (
	MinFillAmount    int64 = 10_000        // smallest fill-up / request the apps allow
	MaxRequestAmount int64 = 2_000_000     // largest limit increase a driver may request
	MaxMoney         int64 = 1_000_000_000 // sanity ceiling for any single amount
)

// Plate is a Sudanese plate: digits plus the state code in Latin and Arabic
// script, e.g. {"4410", "KRT", "خ ر ط"}.
type Plate struct {
	Digits string `json:"digits"`
	Latin  string `json:"latin"`
	Arabic string `json:"arabic"`
}

var plateDigits = regexp.MustCompile(`^[0-9]{1,5}$`)

// latinToArabic maps Latin state-code letters to their Arabic plate letters
// (same table as the prototype's autoAr helper).
var latinToArabic = map[rune]string{
	'K': "خ", 'H': "ح", 'R': "ر", 'T': "ط", 'O': "أ", 'M': "م", 'B': "ب",
	'S': "س", 'D': "د", 'N': "ن", 'G': "ج", 'A': "ا", 'L': "ل", 'P': "ب",
}

// ArabicStateCode transliterates a Latin state code ("KRT" → "خ ر ط").
// Unknown letters are kept as-is.
func ArabicStateCode(latin string) string {
	parts := make([]string, 0, len(latin))
	for _, r := range strings.ToUpper(latin) {
		if a, ok := latinToArabic[r]; ok {
			parts = append(parts, a)
		} else {
			parts = append(parts, string(r))
		}
	}
	return strings.Join(parts, " ")
}

// Normalize trims the plate, upper-cases the Latin code and fills in the
// Arabic code from the Latin one when missing. ok is false when the digits
// or Latin code are invalid.
func (p Plate) Normalize() (Plate, bool) {
	out := Plate{
		Digits: strings.TrimSpace(p.Digits),
		Latin:  strings.ToUpper(strings.TrimSpace(p.Latin)),
		Arabic: strings.TrimSpace(p.Arabic),
	}
	if !plateDigits.MatchString(out.Digits) || out.Latin == "" || len(out.Latin) > 5 {
		return Plate{}, false
	}
	if out.Arabic == "" {
		out.Arabic = ArabicStateCode(out.Latin)
	}
	return out, true
}

// String renders the plate in Latin form ("4410 KRT").
func (p Plate) String() string { return p.Digits + " " + p.Latin }
