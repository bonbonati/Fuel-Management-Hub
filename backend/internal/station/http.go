package station

import (
	"net/http"
	"strconv"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
)

// API exposes the station service over HTTP.
type API struct{ svc *Service }

// NewAPI returns the HTTP adapter for svc.
func NewAPI(svc *Service) *API { return &API{svc: svc} }

// Register adds the routes to mux.
//
// Reads of stations, pumps and prices are open to every signed-in role (the
// driver app lists nearby stations and pumps). Changes are limited to the
// station's own admin; the station comes from the token, and a path id of
// another station is rejected.
func (a *API) Register(mux *http.ServeMux) {
	all := []authn.Role{authn.RoleDriver, authn.RoleFleetManager, authn.RoleAttendant, authn.RoleStationAdmin}
	admin := authn.RoleStationAdmin
	const p = "/api/v1/stations"

	mux.HandleFunc("GET "+p, authn.Require(a.list, all...))
	mux.HandleFunc("GET "+p+"/fuel-prices", authn.Require(a.prices, all...))
	mux.HandleFunc("GET "+p+"/staff/me", authn.Require(a.staffMe, authn.RoleAttendant, admin))
	mux.HandleFunc("GET "+p+"/{id}", authn.Require(a.get, all...))
	mux.HandleFunc("PATCH "+p+"/{id}", authn.Require(a.own(a.patch), admin))
	mux.HandleFunc("GET "+p+"/{id}/pumps", authn.Require(a.pumps, all...))
	mux.HandleFunc("PUT "+p+"/{id}/pumps/{n}", authn.Require(a.own(a.putPump), admin))
	mux.HandleFunc("PATCH "+p+"/{id}/pumps/{n}/status", authn.Require(a.own(a.pumpStatus), admin))
	mux.HandleFunc("GET "+p+"/{id}/staff", authn.Require(a.own(a.staff), admin))
	mux.HandleFunc("POST "+p+"/{id}/staff", authn.Require(a.own(a.saveStaff), admin))
	mux.HandleFunc("PUT "+p+"/{id}/staff/{sid}", authn.Require(a.own(a.saveStaff), admin))
	mux.HandleFunc("PATCH "+p+"/{id}/staff/{sid}/status", authn.Require(a.own(a.staffStatus), admin))

	// Internal: pump + prices for the fueling service.
	mux.HandleFunc("GET /internal/v1/stations/{id}/pumps/{n}/quote", authn.Require(a.quote, all...))
}

// own restricts h to callers whose token belongs to the {id} station.
func (a *API) own(h httpx.HandlerFunc) httpx.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		if authn.Caller(r).Station != r.PathValue("id") {
			return httpx.ErrForbidden
		}
		return h(w, r)
	}
}

func pumpNumber(r *http.Request) (int, error) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n <= 0 {
		return 0, errNotFound
	}
	return n, nil
}

func (a *API) list(w http.ResponseWriter, r *http.Request) error {
	ss, err := a.svc.Stations(r.Context())
	if err != nil {
		return err
	}
	return httpx.OK(w, ss)
}

func (a *API) prices(w http.ResponseWriter, r *http.Request) error {
	p, err := a.svc.Prices(r.Context())
	if err != nil {
		return err
	}
	return httpx.OK(w, p)
}

func (a *API) get(w http.ResponseWriter, r *http.Request) error {
	s, err := a.svc.Station(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	return httpx.OK(w, s)
}

func (a *API) patch(w http.ResponseWriter, r *http.Request) error {
	var in StationPatch
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	s, err := a.svc.UpdateStation(r.Context(), r.PathValue("id"), in)
	if err != nil {
		return err
	}
	return httpx.OK(w, s)
}

func (a *API) pumps(w http.ResponseWriter, r *http.Request) error {
	ps, err := a.svc.Pumps(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	return httpx.OK(w, ps)
}

func (a *API) putPump(w http.ResponseWriter, r *http.Request) error {
	n, err := pumpNumber(r)
	if err != nil {
		return err
	}
	var in PumpInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	p, err := a.svc.UpdatePump(r.Context(), r.PathValue("id"), n, in)
	if err != nil {
		return err
	}
	return httpx.OK(w, p)
}

func (a *API) pumpStatus(w http.ResponseWriter, r *http.Request) error {
	n, err := pumpNumber(r)
	if err != nil {
		return err
	}
	var in struct {
		Online bool `json:"online"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	p, err := a.svc.SetPumpOnline(r.Context(), r.PathValue("id"), n, in.Online)
	if err != nil {
		return err
	}
	return httpx.OK(w, p)
}

func (a *API) quote(w http.ResponseWriter, r *http.Request) error {
	n, err := pumpNumber(r)
	if err != nil {
		return err
	}
	q, err := a.svc.Quote(r.Context(), r.PathValue("id"), n)
	if err != nil {
		return err
	}
	return httpx.OK(w, q)
}

func (a *API) staff(w http.ResponseWriter, r *http.Request) error {
	s, err := a.svc.StaffList(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	return httpx.OK(w, s)
}

func (a *API) saveStaff(w http.ResponseWriter, r *http.Request) error {
	var in StaffInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	s, err := a.svc.SaveStaff(r.Context(), r.PathValue("id"), r.PathValue("sid"), in)
	if err != nil {
		return err
	}
	if r.Method == http.MethodPost {
		return httpx.Created(w, s)
	}
	return httpx.OK(w, s)
}

func (a *API) staffStatus(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Active bool `json:"active"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	s, err := a.svc.SetStaffActive(r.Context(), r.PathValue("id"), r.PathValue("sid"), in.Active)
	if err != nil {
		return err
	}
	return httpx.OK(w, s)
}

// StaffProfile is the signed-in staff member with their station.
type StaffProfile struct {
	Staff   Staff   `json:"staff"`
	Station Station `json:"station"`
}

func (a *API) staffMe(w http.ResponseWriter, r *http.Request) error {
	c := authn.Caller(r)
	st, err := a.svc.StaffMember(r.Context(), c.Station, c.Actor)
	if err != nil {
		return err
	}
	stn, err := a.svc.Station(r.Context(), c.Station)
	if err != nil {
		return err
	}
	return httpx.OK(w, StaffProfile{Staff: st, Station: stn})
}
