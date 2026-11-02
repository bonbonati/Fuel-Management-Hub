package fleet

import (
	"strings"
	"testing"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
)

func TestLimits(t *testing.T) {
	tier := &Tier{Budget: 1_200_000}
	cases := []struct {
		name      string
		v         Vehicle
		t         *Tier
		base, lim int64
	}{
		{"tier budget", Vehicle{}, tier, 1_200_000, 1_200_000},
		{"tier budget plus approved extra", Vehicle{Extra: 150_000}, tier, 1_200_000, 1_350_000},
		{"custom limit overrides tier", Vehicle{CustomLimit: ptr(int64(1_800_000))}, tier, 1_800_000, 1_800_000},
		{"custom limit of zero is respected", Vehicle{CustomLimit: ptr(int64(0)), Extra: 10}, tier, 0, 10},
		{"no tier and no custom", Vehicle{Extra: 5}, nil, 0, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BaseLimit(tc.v, tc.t); got != tc.base {
				t.Errorf("BaseLimit = %d, want %d", got, tc.base)
			}
			if got := Limit(tc.v, tc.t); got != tc.lim {
				t.Errorf("Limit = %d, want %d", got, tc.lim)
			}
			if v := View(tc.v, tc.t); v.Limit != tc.lim || v.BaseLimit != tc.base {
				t.Errorf("View = %+v", v)
			}
		})
	}
}

func TestComputeBlock(t *testing.T) {
	active, suspended := &Driver{Active: true}, &Driver{Active: false}
	veh, vehOff := &Vehicle{Active: true}, &Vehicle{Active: false}
	cases := []struct {
		name string
		d    *Driver
		v    *Vehicle
		want BlockKind
	}{
		{"all active", active, veh, ""},
		{"driver suspended wins over everything", suspended, vehOff, BlockDriver},
		{"suspended driver without vehicle", suspended, nil, BlockDriver},
		{"no vehicle", active, nil, BlockNoVehicle},
		{"vehicle suspended", active, vehOff, BlockVehicle},
		{"vehicle without driver, active", nil, veh, ""},
		{"vehicle without driver, suspended", nil, vehOff, BlockVehicle},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := ComputeBlock(tc.d, tc.v)
			got := BlockKind("")
			if b != nil {
				got = b.Kind
				if !b.Message.Complete() {
					t.Error("block message must be bilingual")
				}
			}
			if got != tc.want {
				t.Fatalf("block = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizePhone(t *testing.T) {
	good := map[string]string{
		"+249 90 552 4471": "+249 90 552 4471",
		"249905524471":     "+249 90 552 4471",
		"+249-90-552-4471": "+249 90 552 4471",
	}
	for in, want := range good {
		got, err := NormalizePhone(in)
		if err != nil || got != want {
			t.Errorf("NormalizePhone(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "+249 ", "+966 50 123 4567", "+249 90 552 447", "+249 90 552 44711"} {
		if _, err := NormalizePhone(bad); err == nil {
			t.Errorf("NormalizePhone(%q) should fail", bad)
		}
	}
}

func TestValidateRequest(t *testing.T) {
	blockDriver := &Block{Kind: BlockDriver}
	blockNoVeh := &Block{Kind: BlockNoVehicle}
	limit := RequestInput{Type: RequestLimit, Amount: 200_000, Reason: "trip"}
	activate := RequestInput{Type: RequestActivate, Reason: "review"}
	pendingLimit := []Request{{Type: RequestLimit, Status: StatusPending}}
	decidedLimit := []Request{{Type: RequestLimit, Status: StatusApproved}}

	cases := []struct {
		name     string
		in       RequestInput
		block    *Block
		pending  []Request
		wantCode string
	}{
		{"valid limit", limit, nil, nil, ""},
		{"limit at lower bound", RequestInput{Type: RequestLimit, Amount: 10_000, Reason: "extra"}, nil, nil, ""},
		{"limit at upper bound", RequestInput{Type: RequestLimit, Amount: 2_000_000, Reason: "extra"}, nil, nil, ""},
		{"limit below range", RequestInput{Type: RequestLimit, Amount: 9_999, Reason: "trip"}, nil, nil, "invalid_amount"},
		{"limit above range", RequestInput{Type: RequestLimit, Amount: 2_000_001, Reason: "trip"}, nil, nil, "invalid_amount"},
		{"limit while suspended", limit, blockDriver, nil, "limit_unavailable"},
		{"activate while suspended", activate, blockDriver, nil, ""},
		{"activate when active", activate, nil, nil, "nothing_to_activate"},
		{"activate with only no-vehicle block", activate, blockNoVeh, nil, "nothing_to_activate"},
		{"reason not allowed for type", RequestInput{Type: RequestLimit, Amount: 50_000, Reason: "review"}, nil, nil, "invalid_reason"},
		{"unknown type", RequestInput{Type: "gift", Reason: "other"}, nil, nil, "invalid_type"},
		{"note too long", RequestInput{Type: RequestLimit, Amount: 50_000, Reason: "trip", Note: strings.Repeat("ن", 281)}, nil, nil, "note_too_long"},
		{"duplicate pending of same type", limit, nil, pendingLimit, "duplicate_request"},
		{"decided request does not block", limit, nil, decidedLimit, ""},
		{"pending of other type does not block", activate, blockDriver, pendingLimit, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRequest(tc.in, tc.block, tc.pending)
			got := ""
			if err != nil {
				got = httpx.AsError(err).Code
			}
			if got != tc.wantCode {
				t.Fatalf("code = %q, want %q (err %v)", got, tc.wantCode, err)
			}
		})
	}
}
