// Command station runs the station service (stations, pumps, staff, fuel
// prices). See internal/station.
package main

import (
	"context"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/app"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/db"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
	"github.com/bonbonati/fuel-management-hub/backend/internal/station"
)

func main() {
	app.Main("station", func(ctx context.Context) error {
		pool, err := app.Database(ctx, station.Migrations, station.Seed)
		if err != nil {
			return err
		}
		defer pool.Close()
		api := station.NewAPI(station.NewService(station.NewPG(pool)))
		return app.Serve(ctx, app.Verifier(), map[string]httpx.Check{"postgres": db.Ping(pool)}, api.Register)
	})
}
