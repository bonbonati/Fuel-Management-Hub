package fueling

import (
	"net/http"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
)

// API exposes the fueling service over HTTP.
type API struct{ svc *Service }

// NewAPI returns the HTTP adapter for svc.
func NewAPI(svc *Service) *API { return &API{svc: svc} }

// Register adds the routes to mux.
func (a *API) Register(mux *http.ServeMux) {
	drv, fm := authn.RoleDriver, authn.RoleFleetManager
	staff := []authn.Role{authn.RoleAttendant, authn.RoleStationAdmin}
	const p = "/api/v1/fueling"

	// Driver app.
	mux.HandleFunc("GET "+p+"/me/pass", authn.Require(a.passStatus, drv))
	mux.HandleFunc("POST "+p+"/me/pass-token", authn.Require(a.passToken, drv))
	mux.HandleFunc("POST "+p+"/me/odometer", authn.Require(a.odometer, drv))
	mux.HandleFunc("POST "+p+"/authorizations", authn.Require(a.authorize, drv))

	// Station (attendant scanner + station admin console).
	mux.HandleFunc("POST "+p+"/scans", authn.Require(a.scan, staff...))
	mux.HandleFunc("POST "+p+"/dispenses", authn.Require(a.dispense, staff...))
	mux.HandleFunc("POST "+p+"/transactions/start-by-code", authn.Require(a.startByCode, staff...))
	mux.HandleFunc("POST "+p+"/transactions/{id}/start", authn.Require(a.start, staff...))
	mux.HandleFunc("POST "+p+"/transactions/{id}/finish", authn.Require(a.finish, staff...))

	// Shared reads (scoped by role in the service).
	mux.HandleFunc("GET "+p+"/transactions", authn.Require(a.list, drv, fm, authn.RoleAttendant, authn.RoleStationAdmin))
	mux.HandleFunc("GET "+p+"/security-events", authn.Require(a.securityEvents, fm))
}

func (a *API) passStatus(w http.ResponseWriter, r *http.Request) error {
	st, err := a.svc.PassStatus(r.Context(), authn.Caller(r).Actor)
	if err != nil {
		return err
	}
	return httpx.OK(w, st)
}

func (a *API) passToken(w http.ResponseWriter, r *http.Request) error {
	tok, err := a.svc.IssuePass(r.Context(), authn.Caller(r).Actor)
	if err != nil {
		return err
	}
	return httpx.Created(w, tok)
}

func (a *API) odometer(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Km string `json:"km"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	st, err := a.svc.RecordOdometer(r.Context(), authn.Caller(r).Actor, in.Km)
	if err != nil {
		return err
	}
	return httpx.Created(w, st)
}

func (a *API) authorize(w http.ResponseWriter, r *http.Request) error {
	var in AuthorizeInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	t, err := a.svc.Authorize(r.Context(), authn.Caller(r).Actor, in)
	if err != nil {
		return err
	}
	return httpx.Created(w, t)
}

func (a *API) scan(w http.ResponseWriter, r *http.Request) error {
	var in ScanInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	res, err := a.svc.Scan(r.Context(), authn.Caller(r), in)
	if err != nil {
		return err
	}
	return httpx.Created(w, res)
}

func (a *API) dispense(w http.ResponseWriter, r *http.Request) error {
	var in DispenseInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	t, err := a.svc.Dispense(r.Context(), authn.Caller(r), in)
	if err != nil {
		return err
	}
	return httpx.Created(w, t)
}

func (a *API) start(w http.ResponseWriter, r *http.Request) error {
	t, err := a.svc.Start(r.Context(), authn.Caller(r), r.PathValue("id"))
	if err != nil {
		return err
	}
	return httpx.OK(w, t)
}

func (a *API) startByCode(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Code string `json:"code"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	t, err := a.svc.StartByCode(r.Context(), authn.Caller(r), in.Code)
	if err != nil {
		return err
	}
	return httpx.OK(w, t)
}

func (a *API) finish(w http.ResponseWriter, r *http.Request) error {
	t, err := a.svc.Finish(r.Context(), authn.Caller(r), r.PathValue("id"))
	if err != nil {
		return err
	}
	return httpx.OK(w, t)
}

func (a *API) list(w http.ResponseWriter, r *http.Request) error {
	ts, err := a.svc.Transactions(r.Context(), authn.Caller(r))
	if err != nil {
		return err
	}
	if ts == nil {
		ts = []Transaction{}
	}
	return httpx.OK(w, ts)
}

func (a *API) securityEvents(w http.ResponseWriter, r *http.Request) error {
	es, err := a.svc.SecurityEvents(r.Context(), authn.Caller(r).Org)
	if err != nil {
		return err
	}
	if es == nil {
		es = []SecurityEvent{}
	}
	return httpx.OK(w, es)
}
