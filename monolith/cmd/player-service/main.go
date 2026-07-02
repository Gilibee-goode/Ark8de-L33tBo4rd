// player-service — class, skills, gear, kredits, stats. Serves /api/players/*
// and /api/skills. Publishes player.gear_updated events to NATS.
package main

import (
	"context"

	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/app"
)

func main() {
	app.Bootstrap()
	pool := app.MustConnectDB(context.Background())
	defer pool.Close()

	natsConn, pub := app.MaybeNATS()
	if natsConn != nil {
		defer natsConn.Close()
	}

	r := app.BuildPlayerRouter(pool, app.MustEnv("JWT_SECRET"), pub)
	app.RunServer(r, app.EnvOr("PORT", "8082"))
}
