package fleet

import (
	"net/http"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
)

// API exposes the fleet service over HTTP.
//
// Public routes (/api/v1/fleet/...) are reachable through the gateway.
// Internal routes (/internal/v1/fleet/...) are only reachable inside the
// cluster and are called by the fueling service with the end user's token.
type API struct{ svc *Service }

// NewAPI returns the HTTP adapter for svc.
func NewAPI(svc *Service) *API { return &API{svc: svc} }

// Register adds the routes to mux. Role requirements are listed per route.
func (a *API) Register(mux *http.ServeMux) {
	fm, drv := authn.RoleFleetManager, authn.RoleDriver
	const p = "/api/v1/fleet"

	mux.HandleFunc("GET "+p+"/org", authn.Require(a.getOrg, fm, drv))
	mux.HandleFunc("PATCH "+p+"/org", authn.Require(a.patchOrg, fm))

	mux.HandleFunc("GET "+p+"/tiers", authn.Require(a.listTiers, fm))
	mux.HandleFunc("POST "+p+"/tiers", authn.Require(a.saveTier, fm))
	mux.HandleFunc("PUT "+p+"/tiers/{id}", authn.Require(a.saveTier, fm))

	mux.HandleFunc("GET "+p+"/drivers", authn.Require(a.listDrivers, fm))
	mux.HandleFunc("POST "+p+"/drivers", authn.Require(a.saveDriver, fm))
	mux.HandleFunc("PUT "+p+"/drivers/{id}", authn.Require(a.saveDriver, fm))
	mux.HandleFunc("PATCH "+p+"/drivers/{id}/status", authn.Require(a.driverStatus, fm))

	mux.HandleFunc("GET "+p+"/vehicles", authn.Require(a.listVehicles, fm))
	mux.HandleFunc("POST "+p+"/vehicles", authn.Require(a.saveVehicle, fm))
	mux.HandleFunc("PUT "+p+"/vehicles/{id}", authn.Require(a.saveVehicle, fm))
	mux.HandleFunc("PATCH "+p+"/vehicles/{id}/status", authn.Require(a.vehicleStatus, fm))
	mux.HandleFunc("PATCH "+p+"/vehicles/{id}/tier", authn.Require(a.vehicleTier, fm))
	mux.HandleFunc("POST "+p+"/vehicles/{id}/limit-increases", authn.Require(a.limitIncrease, fm))

	mux.HandleFunc("GET "+p+"/rules", authn.Require(a.getRules, fm, drv))
	mux.HandleFunc("PATCH "+p+"/rules", authn.Require(a.patchRules, fm))

	mux.HandleFunc("GET "+p+"/requests", authn.Require(a.listRequests, fm, drv))
	mux.HandleFunc("POST "+p+"/requests", authn.Require(a.createRequest, drv))
	mux.HandleFunc("POST "+p+"/requests/{id}/decision", authn.Require(a.decide, fm))

	mux.HandleFunc("GET "+p+"/me", authn.Require(a.me, drv))

	// Internal: authorization contexts for the fueling service.
	mux.HandleFunc("GET /internal/v1/fleet/drivers/{id}/context",
		authn.Require(a.driverContext, drv, authn.RoleAttendant, authn.RoleStationAdmin))
	mux.HandleFunc("GET /internal/v1/fleet/vehicles/{id}/context",
		authn.Require(a.vehicleContext, drv, authn.RoleAttendant, authn.RoleStationAdmin))
}

// ---------- org ----------

func (a *API) getOrg(w http.ResponseWriter, r *http.Request) error {
	o, err := a.svc.Org(r.Context(), authn.Caller(r).Org)
	if err != nil {
		return err
	}
	return httpx.OK(w, o)
}

func (a *API) patchOrg(w http.ResponseWriter, r *http.Request) error {
	var in OrgPatch
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	o, err := a.svc.UpdateOrg(r.Context(), authn.Caller(r).Org, in)
	if err != nil {
		return err
	}
	return httpx.OK(w, o)
}

// ---------- tiers ----------

func (a *API) listTiers(w http.ResponseWriter, r *http.Request) error {
	ts, err := a.svc.Tiers(r.Context(), authn.Caller(r).Org)
	if err != nil {
		return err
	}
	return httpx.OK(w, ts)
}

func (a *API) saveTier(w http.ResponseWriter, r *http.Request) error {
	var in TierInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	t, err := a.svc.SaveTier(r.Context(), authn.Caller(r).Org, r.PathValue("id"), in)
	if err != nil {
		return err
	}
	return created(w, r, t)
}

// created answers 201 for POST (create) and 200 for PUT (replace).
func created(w http.ResponseWriter, r *http.Request, v any) error {
	if r.Method == http.MethodPost {
		return httpx.Created(w, v)
	}
	return httpx.OK(w, v)
}

// ---------- drivers ----------

func (a *API) listDrivers(w http.ResponseWriter, r *http.Request) error {
	ds, err := a.svc.Drivers(r.Context(), authn.Caller(r).Org)
	if err != nil {
		return err
	}
	return httpx.OK(w, ds)
}

func (a *API) saveDriver(w http.ResponseWriter, r *http.Request) error {
	var in DriverInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	d, err := a.svc.SaveDriver(r.Context(), authn.Caller(r).Org, r.PathValue("id"), in)
	if err != nil {
		return err
	}
	return created(w, r, d)
}

// statusInput toggles active/suspended.
type statusInput struct {
	Active bool `json:"active"`
}

func (a *API) driverStatus(w http.ResponseWriter, r *http.Request) error {
	var in statusInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	d, err := a.svc.SetDriverActive(r.Context(), authn.Caller(r).Org, r.PathValue("id"), in.Active)
	if err != nil {
		return err
	}
	return httpx.OK(w, d)
}

// ---------- vehicles ----------

func (a *API) listVehicles(w http.ResponseWriter, r *http.Request) error {
	vs, err := a.svc.Vehicles(r.Context(), authn.Caller(r).Org)
	if err != nil {
		return err
	}
	return httpx.OK(w, vs)
}

func (a *API) saveVehicle(w http.ResponseWriter, r *http.Request) error {
	var in VehicleInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	v, err := a.svc.SaveVehicle(r.Context(), authn.Caller(r).Org, r.PathValue("id"), in)
	if err != nil {
		return err
	}
	return created(w, r, v)
}

func (a *API) vehicleStatus(w http.ResponseWriter, r *http.Request) error {
	var in statusInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	v, err := a.svc.SetVehicleActive(r.Context(), authn.Caller(r).Org, r.PathValue("id"), in.Active)
	if err != nil {
		return err
	}
	return httpx.OK(w, v)
}

func (a *API) vehicleTier(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		TierID *string `json:"tierId"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	tier := ""
	if in.TierID != nil {
		tier = *in.TierID
	}
	v, err := a.svc.SetVehicleTier(r.Context(), authn.Caller(r).Org, r.PathValue("id"), tier)
	if err != nil {
		return err
	}
	return httpx.OK(w, v)
}

func (a *API) limitIncrease(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Amount int64 `json:"amount"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	v, err := a.svc.IncreaseLimit(r.Context(), authn.Caller(r).Org, r.PathValue("id"), in.Amount)
	if err != nil {
		return err
	}
	return httpx.OK(w, v)
}

// ---------- rules ----------

func (a *API) getRules(w http.ResponseWriter, r *http.Request) error {
	x, err := a.svc.Rules(r.Context(), authn.Caller(r).Org)
	if err != nil {
		return err
	}
	return httpx.OK(w, x)
}

func (a *API) patchRules(w http.ResponseWriter, r *http.Request) error {
	var in RulesPatch
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	x, err := a.svc.PatchRules(r.Context(), authn.Caller(r).Org, in)
	if err != nil {
		return err
	}
	return httpx.OK(w, x)
}

// ---------- requests ----------

// listRequests: fleet managers see the whole org (optional ?status=);
// drivers only ever see their own requests.
func (a *API) listRequests(w http.ResponseWriter, r *http.Request) error {
	c := authn.Caller(r)
	driver := ""
	if c.Role == authn.RoleDriver {
		driver = c.Actor
	}
	rs, err := a.svc.Requests(r.Context(), c.Org, r.URL.Query().Get("status"), driver)
	if err != nil {
		return err
	}
	return httpx.OK(w, rs)
}

func (a *API) createRequest(w http.ResponseWriter, r *http.Request) error {
	var in RequestInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	c := authn.Caller(r)
	q, err := a.svc.CreateRequest(r.Context(), c.Org, c.Actor, in)
	if err != nil {
		return err
	}
	return httpx.Created(w, q)
}

func (a *API) decide(w http.ResponseWriter, r *http.Request) error {
	var in DecisionInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	q, err := a.svc.Decide(r.Context(), authn.Caller(r).Org, r.PathValue("id"), in)
	if err != nil {
		return err
	}
	return httpx.OK(w, q)
}

// ---------- contexts ----------

// me returns the calling driver's context (profile, vehicle, tier, rules, block).
func (a *API) me(w http.ResponseWriter, r *http.Request) error {
	c, err := a.svc.DriverContext(r.Context(), authn.Caller(r).Actor)
	if err != nil {
		return err
	}
	return httpx.OK(w, c)
}

// driverContext: a driver may only read their own context; station staff
// may read any driver's (needed to authorize a scanned pass).
func (a *API) driverContext(w http.ResponseWriter, r *http.Request) error {
	c := authn.Caller(r)
	id := r.PathValue("id")
	if c.Role == authn.RoleDriver && c.Actor != id {
		return httpx.ErrForbidden
	}
	ctx, err := a.svc.DriverContext(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.OK(w, ctx)
}

func (a *API) vehicleContext(w http.ResponseWriter, r *http.Request) error {
	c := authn.Caller(r)
	ctx, err := a.svc.VehicleContext(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	// A driver may only read the vehicle assigned to them.
	if c.Role == authn.RoleDriver && (ctx.Driver == nil || ctx.Driver.ID != c.Actor) {
		return httpx.ErrForbidden
	}
	return httpx.OK(w, ctx)
}
