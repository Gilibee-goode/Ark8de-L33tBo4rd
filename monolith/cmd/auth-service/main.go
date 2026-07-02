// auth-service — register, login, JWT issuance. Serves /auth/*.
package main

import (
	"context"

	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/app"
)

func main() {
	app.Bootstrap()
	pool := app.MustConnectDB(context.Background())
	defer pool.Close()

	r := app.BuildAuthRouter(pool, app.MustEnv("JWT_SECRET"))
	app.RunServer(r, app.EnvOr("PORT", "8081"))
}
