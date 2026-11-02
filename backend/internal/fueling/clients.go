package fueling

import (
	"context"
	"net/url"
	"strconv"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/svcclient"
)

// HTTP adapters for the upstream services. Each call forwards the end
// user's token (see svcclient), so upstream services authorize the same
// identity that called fueling.

// FleetClient implements Fleet over HTTP.
type FleetClient struct{ c *svcclient.Client }

// NewFleetClient returns a client for the fleet service at baseURL.
func NewFleetClient(baseURL string) FleetClient { return FleetClient{svcclient.New(baseURL)} }

func (f FleetClient) DriverContext(ctx context.Context, id string) (FleetContext, error) {
	var out FleetContext
	err := f.c.Get(ctx, "/internal/v1/fleet/drivers/"+url.PathEscape(id)+"/context", &out)
	return out, err
}

func (f FleetClient) VehicleContext(ctx context.Context, id string) (FleetContext, error) {
	var out FleetContext
	err := f.c.Get(ctx, "/internal/v1/fleet/vehicles/"+url.PathEscape(id)+"/context", &out)
	return out, err
}

// StationClient implements Stations over HTTP.
type StationClient struct{ c *svcclient.Client }

// NewStationClient returns a client for the station service at baseURL.
func NewStationClient(baseURL string) StationClient { return StationClient{svcclient.New(baseURL)} }

func (s StationClient) Quote(ctx context.Context, stationID string, pump int) (PumpQuote, error) {
	var out PumpQuote
	err := s.c.Get(ctx, "/internal/v1/stations/"+url.PathEscape(stationID)+"/pumps/"+strconv.Itoa(pump)+"/quote", &out)
	return out, err
}

func (s StationClient) Station(ctx context.Context, stationID string) (StationInfo, error) {
	var out StationInfo
	err := s.c.Get(ctx, "/api/v1/stations/"+url.PathEscape(stationID), &out)
	return out, err
}

// PaymentClient implements Payments over HTTP.
type PaymentClient struct{ c *svcclient.Client }

// NewPaymentClient returns a client for the payment service at baseURL.
func NewPaymentClient(baseURL string) PaymentClient { return PaymentClient{svcclient.New(baseURL)} }

func (p PaymentClient) Allowance(ctx context.Context, orgID, vehicleID string) (Allowance, error) {
	var out Allowance
	q := url.Values{"orgId": {orgID}, "vehicleId": {vehicleID}}
	err := p.c.Get(ctx, "/internal/v1/payments/allowance?"+q.Encode(), &out)
	return out, err
}

func (p PaymentClient) Charge(ctx context.Context, in ChargeRequest) error {
	return p.c.Post(ctx, "/internal/v1/payments/charges", in, nil)
}

func (p PaymentClient) Refund(ctx context.Context, txID string) error {
	return p.c.Post(ctx, "/internal/v1/payments/refunds", map[string]string{"transactionId": txID}, nil)
}
