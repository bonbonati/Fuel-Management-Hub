// Command fueling runs the fueling service (pass tokens, scans,
// authorizations, transaction lifecycle, security log) and its outbox relay.
// See internal/fueling.
package main

import (
	"context"
	"time"

	"github.com/bonbonati/fuel-management-hub/backend/internal/fueling"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/app"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/config"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/db"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/events"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/kv"
)

func main() {
	app.Main("fueling", func(ctx context.Context) error {
		amqpURL, err := config.Required("AMQP_URL")
		if err != nil {
			return err
		}
		cache, err := kv.NewValkey(config.String("VALKEY_ADDR", "valkey:6379"), config.String("VALKEY_PASSWORD", ""))
		if err != nil {
			return err
		}
		defer cache.Close()
		pool, err := app.Database(ctx, fueling.Migrations, fueling.Seed)
		if err != nil {
			return err
		}
		defer pool.Close()

		// Publish outbox events (e.g. transaction completed) to RabbitMQ.
		pub := events.NewAMQPPublisher(amqpURL, config.String("EVENTS_EXCHANGE", events.DefaultExchange))
		defer pub.Close()
		go events.NewRelay(pool, pub, time.Second).Run(ctx)

		svc := fueling.NewService(fueling.NewPG(pool), cache,
			fueling.NewFleetClient(config.String("FLEET_URL", "http://fleet:8080")),
			fueling.NewStationClient(config.String("STATION_URL", "http://station:8080")),
			fueling.NewPaymentClient(config.String("PAYMENT_URL", "http://payment:8080")))
		checks := map[string]httpx.Check{"postgres": db.Ping(pool), "valkey": cache.Ping}
		return app.Serve(ctx, app.Verifier(), checks, fueling.NewAPI(svc).Register)
	})
}
