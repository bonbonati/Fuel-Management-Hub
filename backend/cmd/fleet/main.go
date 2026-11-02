// Command fleet runs the fleet service (orgs, tiers, drivers, vehicles,
// rules, driver requests). See internal/fleet.
package main

import (
	"context"

	"github.com/bonbonati/fuel-management-hub/backend/internal/fleet"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/app"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/db"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
)

func main() {
	app.Main("fleet", func(ctx context.Context) error {
		pool, err := app.Database(ctx, fleet.Migrations, fleet.Seed)
		if err != nil {
			return err
		}
		defer pool.Close()
		api := fleet.NewAPI(fleet.NewService(fleet.NewPG(pool)))
		return app.Serve(ctx, app.Verifier(), map[string]httpx.Check{"postgres": db.Ping(pool)}, api.Register)
	})
}
