//go:build e2e

// Package e2e exercises the running system through the public gateway, the
// same way the apps do. It needs the stack up (docker compose up, or the k8s
// dev overlay) and is excluded from normal `go test ./...` by the e2e tag:
//
//	E2E_BASE_URL=http://localhost:8080 go test -tags e2e ./e2e/ -v
//
// The test is written to be re-runnable against the same database: it checks
// deltas rather than absolute seed values and restores what it suspends.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

var base = func() string {
	if v := os.Getenv("E2E_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://localhost:8080"
}()

type obj = map[string]any

// call sends a JSON request and decodes the JSON response into out.
func call(t *testing.T, method, path, token string, body any, out any) int {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, base+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 && strings.Contains(resp.Header.Get("Content-Type"), "json") {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("%s %s: decode %v: %s", method, path, err, raw)
		}
	}
	return resp.StatusCode
}

// must is call that fails the test unless the status matches.
func must(t *testing.T, want int, method, path, token string, body, out any) {
	t.Helper()
	var p obj
	target := out
	if target == nil {
		target = &p
	}
	if got := call(t, method, path, token, body, target); got != want {
		t.Fatalf("%s %s: status %d, want %d (%v)", method, path, got, want, target)
	}
}

func login(t *testing.T, user string) string {
	t.Helper()
	var res struct{ AccessToken string }
	pw := os.Getenv("E2E_PASSWORD")
	if pw == "" {
		pw = "demo1234"
	}
	must(t, 200, "POST", "/api/v1/auth/login", "", obj{"username": user, "password": pw}, &res)
	return res.AccessToken
}

func num(v any) int64 { f, _ := v.(float64); return int64(f) }

func TestFullFuelingJourney(t *testing.T) {
	driver, fm, att, sa := login(t, "D-3071"), login(t, "fleet.manager"), login(t, "ATT-09"), login(t, "ATT-01")

	// Wrong password and anonymous access are rejected.
	if s := call(t, "POST", "/api/v1/auth/login", "", obj{"username": "D-3071", "password": "nope"}, nil); s != 401 {
		t.Fatalf("bad password status %d", s)
	}
	if s := call(t, "GET", "/api/v1/fleet/vehicles", "", nil, nil); s != 401 {
		t.Fatalf("anonymous status %d", s)
	}

	// Driver context comes from the fleet service.
	var me struct {
		Vehicle struct {
			ID    string
			Limit int64
		}
		Block any
	}
	must(t, 200, "GET", "/api/v1/fleet/me", driver, nil, &me)
	if me.Vehicle.ID != "f3" || me.Block != nil {
		t.Fatalf("driver context = %+v", me)
	}

	var wallet0 struct {
		Balance int64
		Usage   map[string]int64
	}
	must(t, 200, "GET", "/api/v1/payments/wallet", fm, nil, &wallet0)

	// --- App authorization: odometer → authorize → start by code → finish.
	must(t, 201, "POST", "/api/v1/fueling/me/odometer", driver, obj{"km": "142880"}, nil)
	var auth struct {
		ID, Code, Status string
		Amount           int64
	}
	must(t, 201, "POST", "/api/v1/fueling/authorizations", driver, obj{"stationId": "s1", "pump": 3, "fuel": "95", "amount": 100000}, &auth)
	if auth.Status != "auth" || len(auth.Code) != 4 {
		t.Fatalf("authorization = %+v", auth)
	}
	// The odometer reading was consumed: a second authorization needs a new one.
	var p obj
	if s := call(t, "POST", "/api/v1/fueling/authorizations", driver, obj{"stationId": "s1", "pump": 3, "fuel": "95", "amount": 100000}, &p); s != 409 || p["code"] != "odometer_required" {
		t.Fatalf("second authorization: %d %v", s, p)
	}

	var wallet1 struct {
		Balance int64
		Usage   map[string]int64
	}
	must(t, 200, "GET", "/api/v1/payments/wallet", fm, nil, &wallet1)
	if wallet0.Balance-wallet1.Balance != 100000 || wallet1.Usage["f3"]-wallet0.Usage["f3"] != 100000 {
		t.Fatalf("charge not reflected: balance %d→%d usage %d→%d", wallet0.Balance, wallet1.Balance, wallet0.Usage["f3"], wallet1.Usage["f3"])
	}

	var settle0 struct{ NextPayout struct{ Amount int64 } }
	must(t, 200, "GET", "/api/v1/payments/settlements", sa, nil, &settle0)

	must(t, 200, "POST", "/api/v1/fueling/transactions/start-by-code", att, obj{"code": auth.Code}, nil)
	must(t, 200, "POST", "/api/v1/fueling/transactions/"+auth.ID+"/finish", att, nil, nil)

	// Completion travels outbox → RabbitMQ → payment consumer → settlements.
	deadline := time.Now().Add(20 * time.Second)
	for {
		var s struct{ NextPayout struct{ Amount int64 } }
		must(t, 200, "GET", "/api/v1/payments/settlements", sa, nil, &s)
		if s.NextPayout.Amount-settle0.NextPayout.Amount == 100000 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("settlement not accrued via events: %d → %d", settle0.NextPayout.Amount, s.NextPayout.Amount)
		}
		time.Sleep(500 * time.Millisecond)
	}

	// --- QR flow: pass token → scan → dispense by litres → finish.
	must(t, 201, "POST", "/api/v1/fueling/me/odometer", driver, obj{"km": "142990"}, nil)
	var pass struct{ Token string }
	must(t, 201, "POST", "/api/v1/fueling/me/pass-token", driver, nil, &pass)
	var scan struct {
		ScanID string
		Active bool
	}
	must(t, 201, "POST", "/api/v1/fueling/scans", att, obj{"method": "qr", "token": pass.Token}, &scan)
	if !scan.Active {
		t.Fatalf("scan = %+v", scan)
	}
	var disp struct {
		ID     string
		Amount int64
		Status string
	}
	must(t, 201, "POST", "/api/v1/fueling/dispenses", att, obj{"scanId": scan.ScanID, "pump": 5, "litres": 20}, &disp)
	if disp.Amount != 72000 || disp.Status != "fueling" { // 20 L × 3,600
		t.Fatalf("dispense = %+v", disp)
	}
	must(t, 200, "POST", "/api/v1/fueling/transactions/"+disp.ID+"/finish", att, nil, nil)

	// --- Driver request → manager approval → higher limit.
	var req struct{ ID string }
	must(t, 201, "POST", "/api/v1/fleet/requests", driver, obj{"type": "limit", "amount": 250000, "reason": "trip", "note": "e2e"}, &req)
	must(t, 200, "POST", "/api/v1/fleet/requests/"+req.ID+"/decision", fm, obj{"approve": true}, nil)
	var me2 struct{ Vehicle struct{ Limit int64 } }
	must(t, 200, "GET", "/api/v1/fleet/me", driver, nil, &me2)
	if me2.Vehicle.Limit-me.Vehicle.Limit != 250000 {
		t.Fatalf("limit %d → %d", me.Vehicle.Limit, me2.Vehicle.Limit)
	}

	// --- Suspension: blocked pass, security log, reactivation request.
	var sec0 []obj
	must(t, 200, "GET", "/api/v1/fueling/security-events", fm, nil, &sec0)
	must(t, 200, "PATCH", "/api/v1/fleet/drivers/d3/status", fm, obj{"active": false}, nil)
	t.Cleanup(func() { call(t, "PATCH", "/api/v1/fleet/drivers/d3/status", fm, obj{"active": true}, nil) })

	must(t, 201, "POST", "/api/v1/fueling/me/pass-token", driver, nil, &pass)
	must(t, 201, "POST", "/api/v1/fueling/scans", att, obj{"method": "qr", "token": pass.Token}, &scan)
	if scan.Active {
		t.Fatal("suspended driver's pass must scan as inactive")
	}
	var sec1 []obj
	must(t, 200, "GET", "/api/v1/fueling/security-events", fm, nil, &sec1)
	if len(sec1) != len(sec0)+1 {
		t.Fatalf("security events %d → %d", len(sec0), len(sec1))
	}
	if s := call(t, "POST", "/api/v1/fleet/requests", driver, obj{"type": "limit", "amount": 50000, "reason": "trip"}, &p); s != 409 {
		t.Fatalf("limit request while suspended: %d", s)
	}
	must(t, 201, "POST", "/api/v1/fleet/requests", driver, obj{"type": "activate", "reason": "review"}, &req)
	must(t, 200, "POST", "/api/v1/fleet/requests/"+req.ID+"/decision", fm, obj{"approve": true, "note": "ok"}, nil)
	must(t, 200, "GET", "/api/v1/fleet/me", driver, nil, &me)
	if me.Block != nil {
		t.Fatalf("driver still blocked after reactivation: %v", me.Block)
	}
}

func TestInternalRoutesAreNotExposed(t *testing.T) {
	driver := login(t, "D-3071")
	for _, path := range []string{"/internal/v1/fleet/drivers/d3/context", "/internal/v1/payments/allowance?orgId=o1&vehicleId=f3"} {
		req, _ := http.NewRequest("GET", base+path, nil)
		req.Header.Set("Authorization", "Bearer "+driver)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 200 && strings.Contains(string(body), "orgId") {
			t.Fatalf("%s is reachable through the gateway", path)
		}
	}
}

func TestStationAdminFlows(t *testing.T) {
	sa := login(t, "ATT-01")
	var pumps []obj
	must(t, 200, "GET", "/api/v1/stations/s1/pumps", sa, nil, &pumps)
	if len(pumps) != 8 {
		t.Fatalf("pumps = %d", len(pumps))
	}
	must(t, 200, "PATCH", "/api/v1/stations/s1/pumps/7/status", sa, obj{"online": true}, nil)
	must(t, 200, "PATCH", "/api/v1/stations/s1/pumps/7/status", sa, obj{"online": false}, nil)
	if s := call(t, "PATCH", "/api/v1/stations/s2/pumps/1/status", sa, obj{"online": false}, nil); s != 403 {
		t.Fatalf("other station's pump: %d", s)
	}
	var tax struct {
		Enabled bool
		Rate    float64
	}
	must(t, 200, "GET", "/api/v1/payments/tax", sa, nil, &tax)
	must(t, 200, "PUT", "/api/v1/payments/tax", sa, obj{"enabled": tax.Enabled, "rate": tax.Rate}, nil)
	fmt.Fprintln(os.Stderr, "station admin flows ok")
}
