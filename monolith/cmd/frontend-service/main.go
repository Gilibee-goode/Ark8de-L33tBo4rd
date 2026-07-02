// frontend-service — server-rendered HTML UI: leaderboard, team detail,
// profile, moderator panel, plus session-based login/register/logout.
package main

import (
	"context"

	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/app"
)

func main() {
	app.Bootstrap()
	pool := app.MustConnectDB(context.Background())
	defer pool.Close()

	r := app.BuildFrontendRouter(pool, app.MustEnv("JWT_SECRET"), app.EnvOr("TEMPLATES_DIR", "templates"))
	app.RunServer(r, app.EnvOr("PORT", "8085"))
}
