package fleet

import (
	"net/http"
	"testing"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn/authntest"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/i18n"
)

func newHTTP(t *testing.T) (*authntest.Env, *authntest.Client, memStore) {
	env := authntest.New(t)
	st := newSeededStore()
	c := env.Server(NewAPI(NewService(st)).Register)
	return env, c, st
}

func TestRoleGuards(t *testing.T) {
	env, c, _ := newHTTP(t)
	fm := env.Token(authn.RoleFleetManager, "o1", "", "m1")
	drv := env.Token(authn.RoleDriver, "o1", "", "d3")
	att := env.Token(authn.RoleAttendant, "", "s1", "a2")

	cases := []struct {
		method, path, token string
		body                any
		want                int
	}{
		{"GET", "/api/v1/fleet/vehicles", "", nil, http.StatusUnauthorized},
		{"GET", "/api/v1/fleet/vehicles", drv, nil, http.StatusForbidden},
		{"GET", "/api/v1/fleet/vehicles", att, nil, http.StatusForbidden},
		{"GET", "/api/v1/fleet/vehicles", fm, nil, http.StatusOK},
		{"GET", "/api/v1/fleet/org", drv, nil, http.StatusOK},
		{"PATCH", "/api/v1/fleet/org", drv, map[string]any{"phone": "1"}, http.StatusForbidden},
		{"GET", "/api/v1/fleet/me", drv, nil, http.StatusOK},
		{"GET", "/api/v1/fleet/me", fm, nil, http.StatusForbidden},
		{"POST", "/api/v1/fleet/requests", fm, map[string]any{"type": "limit"}, http.StatusForbidden},
		{"POST", "/api/v1/fleet/requests/R-1043/decision", drv, map[string]any{"approve": true}, http.StatusForbidden},
		// Internal contexts: drivers only for themselves, station staff for anyone.
		{"GET", "/internal/v1/fleet/drivers/d3/context", drv, nil, http.StatusOK},
		{"GET", "/internal/v1/fleet/drivers/d1/context", drv, nil, http.StatusForbidden},
		{"GET", "/internal/v1/fleet/vehicles/f1/context", drv, nil, http.StatusForbidden},
		{"GET", "/internal/v1/fleet/vehicles/f3/context", drv, nil, http.StatusOK},
		{"GET", "/internal/v1/fleet/vehicles/f6/context", att, nil, http.StatusOK},
		{"GET", "/internal/v1/fleet/vehicles/f6/context", fm, nil, http.StatusForbidden},
	}
	for _, tc := range cases {
		if got := c.Do(tc.method, tc.path, tc.token, tc.body, nil); got != tc.want {
			t.Errorf("%s %s: status %d, want %d", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestDriverRequestRoundTrip(t *testing.T) {
	env, c, st := newHTTP(t)
	drv := env.Token(authn.RoleDriver, "o1", "", "d3")
	fm := env.Token(authn.RoleFleetManager, "o1", "", "m1")

	var created Request
	if s := c.Do("POST", "/api/v1/fleet/requests", drv, RequestInput{Type: RequestLimit, Amount: 200_000, Reason: "trip"}, &created); s != http.StatusCreated {
		t.Fatalf("create status %d", s)
	}
	// Driver identity comes from the token, never from the body.
	if created.DriverID != "d3" || created.OrgID != "o1" {
		t.Fatalf("created = %+v", created)
	}

	var mine []Request
	c.Do("GET", "/api/v1/fleet/requests", drv, nil, &mine)
	for _, r := range mine {
		if r.DriverID != "d3" {
			t.Fatalf("driver saw another driver's request %s", r.ID)
		}
	}

	var pending []Request
	c.Do("GET", "/api/v1/fleet/requests?status=pending", fm, nil, &pending)
	if len(pending) != 2 {
		t.Fatalf("manager sees %d pending, want 2", len(pending))
	}

	var decided Request
	if s := c.Do("POST", "/api/v1/fleet/requests/"+created.ID+"/decision", fm, DecisionInput{Approve: true}, &decided); s != http.StatusOK {
		t.Fatalf("decide status %d", s)
	}
	if decided.Approved != 200_000 || st.st.vehicles["f3"].Extra != 350_000 {
		t.Fatalf("decided = %+v, extra = %d", decided, st.st.vehicles["f3"].Extra)
	}
}

func TestValidationErrorsAreBilingualProblems(t *testing.T) {
	env, c, _ := newHTTP(t)
	fm := env.Token(authn.RoleFleetManager, "o1", "", "m1")

	var p authntest.Problem
	s := c.Do("POST", "/api/v1/fleet/drivers", fm, DriverInput{Name: i18n.T("أ", "A"), Code: "D-1", Phone: "123"}, &p)
	if s != http.StatusBadRequest || p.Code != "invalid_phone" || p.Message.Ar == "" || p.Message.En == "" {
		t.Fatalf("status %d problem %+v", s, p)
	}
	// Unknown JSON fields are rejected.
	if s := c.Do("PATCH", "/api/v1/fleet/rules", fm, map[string]any{"odometr": false}, &p); s != http.StatusBadRequest || p.Code != "invalid_json" {
		t.Fatalf("typo field: status %d code %q", s, p.Code)
	}
}

func TestTenantIsolationOverHTTP(t *testing.T) {
	env, c, _ := newHTTP(t)
	other := env.Token(authn.RoleFleetManager, "o2", "", "m2")

	var vs []VehicleView
	c.Do("GET", "/api/v1/fleet/vehicles", other, nil, &vs)
	if len(vs) != 1 || vs[0].ID != "fx" {
		t.Fatalf("o2 manager sees %+v", vs)
	}
	if s := c.Do("PATCH", "/api/v1/fleet/vehicles/f3/status", other, map[string]bool{"active": false}, nil); s != http.StatusNotFound {
		t.Fatalf("cross-tenant update status %d, want 404", s)
	}
}

func TestMeReturnsDriverContext(t *testing.T) {
	env, c, _ := newHTTP(t)
	var me Context
	c.Do("GET", "/api/v1/fleet/me", env.Token(authn.RoleDriver, "o1", "", "d3"), nil, &me)
	if me.Driver == nil || me.Driver.ID != "d3" || me.Vehicle == nil || me.Vehicle.Limit != 1_350_000 || me.OrgName.En == "" {
		t.Fatalf("me = %+v", me)
	}
}
