// Command payment runs the payment service (wallets, ledger, VAT,
// settlements) and its settlement event consumer. See internal/payment.
package main

import (
	"context"

	"github.com/bonbonati/fuel-management-hub/backend/internal/payment"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/app"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/config"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/db"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/events"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
)

func main() {
	app.Main("payment", func(ctx context.Context) error {
		feeBps, err := config.Int("SETTLEMENT_FEE_BPS", 43) // 0.43% platform fee
		if err != nil {
			return err
		}
		amqpURL, err := config.Required("AMQP_URL")
		if err != nil {
			return err
		}
		pool, err := app.Database(ctx, payment.Migrations, payment.Seed)
		if err != nil {
			return err
		}
		defer pool.Close()
		svc := payment.NewService(payment.NewPG(pool), int64(feeBps))

		// Accrue station settlements from completed fill-ups.
		go events.Consumer{
			URL: amqpURL, Exchange: config.String("EVENTS_EXCHANGE", events.DefaultExchange),
			Queue: "payment.settlement-accrual", Keys: []string{events.TypeTransactionCompleted},
			Handler: svc.SettlementHandler,
		}.Run(ctx)

		api := payment.NewAPI(svc)
		return app.Serve(ctx, app.Verifier(), map[string]httpx.Check{"postgres": db.Ping(pool)}, api.Register)
	})
}
