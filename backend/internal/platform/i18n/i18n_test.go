package i18n

import (
	"encoding/json"
	"testing"
)

func TestTextJSONShape(t *testing.T) {
	b, err := json.Marshal(T("مرحبا", "Hello"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"ar":"مرحبا","en":"Hello"}` {
		t.Fatalf("unexpected JSON %s", b)
	}
}

func TestSameAndPredicates(t *testing.T) {
	s := Same("note")
	if s.Ar != "note" || s.En != "note" {
		t.Fatalf("Same = %+v", s)
	}
	if !T("a", "b").Complete() || T("a", " ").Complete() {
		t.Fatal("Complete must require both languages")
	}
	if !T(" ", "").IsZero() || T("a", "").IsZero() {
		t.Fatal("IsZero must be true only when both are blank")
	}
	if got := T(" a ", " b ").Trimmed(); got != T("a", "b") {
		t.Fatalf("Trimmed = %+v", got)
	}
}
