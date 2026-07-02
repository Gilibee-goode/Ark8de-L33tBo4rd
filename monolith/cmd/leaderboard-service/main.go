// leaderboard-service — read-only team rankings. Serves /api/leaderboard/*.
// Subscribes to team.* events on NATS; rankings are computed live from the
// database, so events are logged for observability (and future caching).
package main

import (
	"context"
	"log/slog"

	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/app"
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/events"
)

func main() {
	app.Bootstrap()
	ctx := context.Background()
	pool := app.MustConnectDB(ctx)
	defer pool.Close()

	natsConn, _ := app.MaybeNATS()
	if natsConn != nil {
		defer natsConn.Close()

		stop, err := natsConn.Subscribe(ctx, "leaderboard-service",
			[]string{events.SubjectTeamPointsUpdated, events.SubjectTeamMembershipChanged},
			func(subject string, data []byte) {
				// The RANK() query reads live data, so there is nothing to
				// recompute — this is the hook where a cached leaderboard
				// would be invalidated.
				slog.Info("leaderboard: event received", "subject", subject, "payload", string(data))
			})
		if err != nil {
			slog.Error("failed to subscribe to events", "error", err)
		} else {
			defer stop()
		}
	}

	r := app.BuildLeaderboardRouter(pool)
	app.RunServer(r, app.EnvOr("PORT", "8084"))
}
