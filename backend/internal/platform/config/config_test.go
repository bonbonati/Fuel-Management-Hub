package config

import (
	"reflect"
	"testing"
	"time"
)

func TestString(t *testing.T) {
	t.Setenv("CFG_STR", "  value  ")
	if got := String("CFG_STR", "def"); got != "value" {
		t.Fatalf("String trimmed = %q, want %q", got, "value")
	}
	t.Setenv("CFG_EMPTY", "   ")
	if got := String("CFG_EMPTY", "def"); got != "def" {
		t.Fatalf("String on blank = %q, want default", got)
	}
	if got := String("CFG_UNSET_FOR_TEST", "def"); got != "def" {
		t.Fatalf("String on unset = %q, want default", got)
	}
}

func TestRequired(t *testing.T) {
	if _, err := Required("CFG_UNSET_FOR_TEST"); err == nil {
		t.Fatal("Required on unset variable should fail")
	}
	t.Setenv("CFG_REQ", "x")
	if v, err := Required("CFG_REQ"); err != nil || v != "x" {
		t.Fatalf("Required = %q, %v", v, err)
	}
}

func TestInt(t *testing.T) {
	tests := []struct {
		name, val string
		want      int
		wantErr   bool
	}{
		{"unset uses default", "", 7, false},
		{"parses number", "42", 42, false},
		{"rejects garbage", "4x", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CFG_INT", tc.val)
			got, err := Int("CFG_INT", 7)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("Int = %d, %v; want %d, err=%v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestBool(t *testing.T) {
	t.Setenv("CFG_BOOL", "true")
	if b, err := Bool("CFG_BOOL", false); err != nil || !b {
		t.Fatalf("Bool(true) = %v, %v", b, err)
	}
	t.Setenv("CFG_BOOL", "nope")
	if _, err := Bool("CFG_BOOL", false); err == nil {
		t.Fatal("Bool should reject malformed values")
	}
	t.Setenv("CFG_BOOL", "")
	if b, _ := Bool("CFG_BOOL", true); !b {
		t.Fatal("Bool should fall back to default")
	}
}

func TestDuration(t *testing.T) {
	t.Setenv("CFG_DUR", "90s")
	if d, err := Duration("CFG_DUR", time.Second); err != nil || d != 90*time.Second {
		t.Fatalf("Duration = %v, %v", d, err)
	}
	t.Setenv("CFG_DUR", "soon")
	if _, err := Duration("CFG_DUR", time.Second); err == nil {
		t.Fatal("Duration should reject malformed values")
	}
}

func TestList(t *testing.T) {
	t.Setenv("CFG_LIST", "a, b,,c ")
	if got := List("CFG_LIST", nil); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("List = %v", got)
	}
	if got := List("CFG_UNSET_FOR_TEST", []string{"d"}); !reflect.DeepEqual(got, []string{"d"}) {
		t.Fatalf("List default = %v", got)
	}
}
