package payment

import (
	"context"
	"net/http"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/events"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
)

// API exposes the payment service over HTTP.
type API struct{ svc *Service }

// NewAPI returns the HTTP adapter for svc.
func NewAPI(svc *Service) *API { return &API{svc: svc} }

// Register adds the routes to mux.
func (a *API) Register(mux *http.ServeMux) {
	fm, sa := authn.RoleFleetManager, authn.RoleStationAdmin
	all := []authn.Role{authn.RoleDriver, fm, authn.RoleAttendant, sa}
	// Roles that may initiate fill-ups (through the fueling service).
	fuelers := []authn.Role{authn.RoleDriver, authn.RoleAttendant, sa}
	const p = "/api/v1/payments"

	mux.HandleFunc("GET "+p+"/wallet", authn.Require(a.wallet, fm))
	mux.HandleFunc("POST "+p+"/wallet/topups", authn.Require(a.topUp, fm))
	mux.HandleFunc("GET "+p+"/tax", authn.Require(a.getTax, all...))
	mux.HandleFunc("PUT "+p+"/tax", authn.Require(a.putTax, fm, sa))
	mux.HandleFunc("GET "+p+"/settlements", authn.Require(a.settlements, sa))

	mux.HandleFunc("GET /internal/v1/payments/allowance", authn.Require(a.allowance, fuelers...))
	mux.HandleFunc("POST /internal/v1/payments/charges", authn.Require(a.charge, fuelers...))
	mux.HandleFunc("POST /internal/v1/payments/refunds", authn.Require(a.refund, fuelers...))
}

func (a *API) wallet(w http.ResponseWriter, r *http.Request) error {
	wl, err := a.svc.Wallet(r.Context(), authn.Caller(r).Org)
	if err != nil {
		return err
	}
	return httpx.OK(w, wl)
}

func (a *API) topUp(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Amount int64 `json:"amount"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	wl, err := a.svc.TopUp(r.Context(), authn.Caller(r).Org, in.Amount)
	if err != nil {
		return err
	}
	return httpx.OK(w, wl)
}

func (a *API) getTax(w http.ResponseWriter, r *http.Request) error {
	t, err := a.svc.Tax(r.Context())
	if err != nil {
		return err
	}
	return httpx.OK(w, t)
}

func (a *API) putTax(w http.ResponseWriter, r *http.Request) error {
	var in Tax
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	t, err := a.svc.SetTax(r.Context(), in)
	if err != nil {
		return err
	}
	return httpx.OK(w, t)
}

func (a *API) settlements(w http.ResponseWriter, r *http.Request) error {
	s, err := a.svc.Settlements(r.Context(), authn.Caller(r).Station)
	if err != nil {
		return err
	}
	return httpx.OK(w, s)
}

// sameOrgDriver rejects drivers acting on another org's wallet. Station
// staff act on whichever fleet's vehicle they serve.
func sameOrgDriver(c *authn.Claims, orgID string) error {
	if c.Role == authn.RoleDriver && c.Org != orgID {
		return errForbidden
	}
	return nil
}

func (a *API) allowance(w http.ResponseWriter, r *http.Request) error {
	org, veh := r.URL.Query().Get("orgId"), r.URL.Query().Get("vehicleId")
	if err := sameOrgDriver(authn.Caller(r), org); err != nil {
		return err
	}
	al, err := a.svc.Allowance(r.Context(), org, veh)
	if err != nil {
		return err
	}
	return httpx.OK(w, al)
}

func (a *API) charge(w http.ResponseWriter, r *http.Request) error {
	var in ChargeInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	if err := sameOrgDriver(authn.Caller(r), in.OrgID); err != nil {
		return err
	}
	res, err := a.svc.Charge(r.Context(), in)
	if err != nil {
		return err
	}
	return httpx.OK(w, res)
}

func (a *API) refund(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		TransactionID string `json:"transactionId"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	if err := a.svc.Refund(r.Context(), in.TransactionID); err != nil {
		return err
	}
	return httpx.OK(w, map[string]bool{"refunded": true})
}

// SettlementHandler consumes "fueling.transaction.completed" events and
// accrues them into the station's next payout.
func (s *Service) SettlementHandler(ctx context.Context, ev events.Event) error {
	var p events.TransactionCompleted
	if err := ev.Decode(&p); err != nil {
		return nil // malformed payload: nothing sensible to retry
	}
	return s.AccrueCompleted(ctx, ev.ID, p.StationID, p.Amount, p.CompletedAt)
}
