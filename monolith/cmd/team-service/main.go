// team-service — team CRUD, join requests, lock-in, arkade points. Serves
// /api/teams/*. Publishes team.points_updated and team.membership_changed.
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

	r := app.BuildTeamRouter(pool, app.MustEnv("JWT_SECRET"), pub)
	app.RunServer(r, app.EnvOr("PORT", "8083"))
}
