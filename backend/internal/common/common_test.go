package common

import "testing"

func TestFuel(t *testing.T) {
	for _, f := range Fuels {
		if !f.Valid() {
			t.Fatalf("%s should be valid", f)
		}
		if DefaultPrices[f] <= 0 {
			t.Fatalf("missing default price for %s", f)
		}
	}
	if Fuel("98").Valid() {
		t.Fatal("unknown grade reported valid")
	}
	if FuelDiesel.PassTag() != "DZ" || Fuel95.PassTag() != "95" {
		t.Fatal("PassTag mismatch with prototype token format")
	}
}

func TestArabicStateCode(t *testing.T) {
	cases := map[string]string{"KRT": "خ ر ط", "kh": "خ ح", "OM": "أ م", "X": "X"}
	for in, want := range cases {
		if got := ArabicStateCode(in); got != want {
			t.Errorf("ArabicStateCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlateNormalize(t *testing.T) {
	p, ok := Plate{Digits: " 4410 ", Latin: "krt"}.Normalize()
	if !ok || p.Digits != "4410" || p.Latin != "KRT" || p.Arabic != "خ ر ط" {
		t.Fatalf("Normalize = %+v, %v", p, ok)
	}
	if p.String() != "4410 KRT" {
		t.Fatalf("String = %q", p.String())
	}
	keep, _ := Plate{Digits: "1", Latin: "KH", Arabic: "خ"}.Normalize()
	if keep.Arabic != "خ" {
		t.Fatal("explicit Arabic code must be kept")
	}
	for _, bad := range []Plate{{Digits: "12a", Latin: "K"}, {Digits: "123456", Latin: "K"}, {Digits: "1", Latin: ""}, {Digits: "", Latin: "K"}} {
		if _, ok := bad.Normalize(); ok {
			t.Errorf("plate %+v should be invalid", bad)
		}
	}
}
