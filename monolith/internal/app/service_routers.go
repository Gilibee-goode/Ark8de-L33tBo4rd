// Package app — per-service router builders (Phase 3).
//
// Each Build*Router function wires the routes for exactly one microservice.
// The route patterns are identical to the monolith's BuildRouter, so the API
// contract does not change — only which binary serves which prefix.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/auth"
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/events"
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/frontend"
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/leaderboard"
	authmw "github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/middleware"
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/player"
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/session"
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/team"
)

// newBaseRouter creates a chi router with the standard middleware stack and
// a /healthz endpoint that pings the database.
func newBaseRouter(pool *pgxpool.Pool) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, req *http.Request) {
		pingCtx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			slog.Error("healthz: database ping failed", "error", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "database unreachable")
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})
	return r
}

// BuildAuthRouter serves /auth/* — register, login, me.
func BuildAuthRouter(pool *pgxpool.Pool, jwtSecret string) chi.Router {
	repo := auth.NewPlayerRepository(pool)
	svc := auth.NewAuthService(repo, jwtSecret)
	h := auth.NewAuthHandler(svc)

	r := newBaseRouter(pool)
	r.Post("/auth/register", h.Register)
	r.Post("/auth/login", h.Login)
	r.With(authmw.Authenticate(jwtSecret)).Get("/auth/me", h.Me)
	return r
}

// BuildPlayerRouter serves /api/skills and /api/players/* — class, skills,
// gear, kredits, stats. Publishes player.gear_updated events.
func BuildPlayerRouter(pool *pgxpool.Pool, jwtSecret string, pub events.Publisher) chi.Router {
	repo := player.NewPlayerRepository(pool)
	svc := player.NewPlayerService(repo).WithEvents(pub)
	h := player.NewPlayerHandler(svc)

	r := newBaseRouter(pool)
	r.Get("/api/skills", h.ListSkills)
	r.Route("/api/players", func(r chi.Router) {
		r.Get("/{id}", h.GetPublicProfile)

		r.With(authmw.Authenticate(jwtSecret)).Put("/me/class", h.SetClass)
		r.With(authmw.Authenticate(jwtSecret)).Get("/me/stats", h.GetStats)
		r.With(authmw.Authenticate(jwtSecret)).Get("/me/skills", h.GetSkills)
		r.With(authmw.Authenticate(jwtSecret)).Put("/me/skills", h.SetSkills)
		r.With(authmw.Authenticate(jwtSecret)).Get("/me/gear", h.GetGear)
		r.With(authmw.Authenticate(jwtSecret)).Put("/me/gear", h.SetGear)
		r.With(authmw.Authenticate(jwtSecret)).Get("/me/kredits", h.GetKredits)
		r.With(authmw.Authenticate(jwtSecret)).Post("/me/kredits/transfer", h.TransferKredits)

		r.With(
			authmw.Authenticate(jwtSecret),
			authmw.RequireRole("moderator", "admin"),
		).Post("/{id}/kredits", h.GrantKredits)
	})
	return r
}

// BuildTeamRouter serves /api/teams/* — CRUD, join requests, lock-in, points.
// Publishes team.points_updated and team.membership_changed events.
func BuildTeamRouter(pool *pgxpool.Pool, jwtSecret string, pub events.Publisher) chi.Router {
	repo := team.NewTeamRepository(pool)
	svc := team.NewTeamService(repo).WithEvents(pub)
	h := team.NewTeamHandler(svc)

	r := newBaseRouter(pool)
	r.Route("/api/teams", func(r chi.Router) {
		r.Get("/", h.ListTeams)
		r.Get("/{id}", h.GetTeam)

		r.With(authmw.Authenticate(jwtSecret)).Post("/", h.CreateTeam)
		r.With(authmw.Authenticate(jwtSecret)).Put("/{id}", h.UpdateTeam)
		r.With(authmw.Authenticate(jwtSecret)).Delete("/{id}", h.DeleteTeam)
		r.With(authmw.Authenticate(jwtSecret)).Put("/{id}/lock", h.ToggleLock)
		r.With(authmw.Authenticate(jwtSecret)).Delete("/{id}/members/{pid}", h.RemoveMember)
		r.With(authmw.Authenticate(jwtSecret)).Post("/{id}/join-requests", h.SendJoinRequest)
		r.With(authmw.Authenticate(jwtSecret)).Get("/{id}/join-requests", h.GetJoinRequests)
		r.With(authmw.Authenticate(jwtSecret)).Put("/{id}/join-requests/{rid}", h.ResolveJoinRequest)
		r.With(authmw.Authenticate(jwtSecret)).Put("/{id}/logo", h.UploadLogo)

		r.With(
			authmw.Authenticate(jwtSecret),
			authmw.RequireRole("moderator", "admin"),
		).Put("/{id}/points", h.AddArkadePoints)
		r.With(
			authmw.Authenticate(jwtSecret),
			authmw.RequireRole("moderator", "admin"),
		).Get("/{id}/points/history", h.GetArkadePointHistory)
	})
	return r
}

// BuildLeaderboardRouter serves /api/leaderboard/* — read-only rankings.
func BuildLeaderboardRouter(pool *pgxpool.Pool) chi.Router {
	repo := leaderboard.NewLeaderboardRepository(pool)
	svc := leaderboard.NewLeaderboardService(repo)
	h := leaderboard.NewLeaderboardHandler(svc)

	r := newBaseRouter(pool)
	r.Get("/api/leaderboard", h.GetLeaderboard)
	r.Get("/api/leaderboard/teams/{id}", h.GetTeamCard)
	return r
}

// BuildFrontendRouter serves the server-rendered HTML pages, static assets,
// and the session-based login/register/logout flows.
//
// NOTE (Phase 3 trade-off): the frontend reads directly from the shared
// database rather than calling the other services over REST. This keeps the
// extraction mechanical; swapping to HTTP clients is a future refactor.
func BuildFrontendRouter(pool *pgxpool.Pool, jwtSecret string, templatesDir string) chi.Router {
	authRepo := auth.NewPlayerRepository(pool)
	authService := auth.NewAuthService(authRepo, jwtSecret)

	playerRepo := player.NewPlayerRepository(pool)
	playerService := player.NewPlayerService(playerRepo)

	teamRepo := team.NewTeamRepository(pool)
	teamService := team.NewTeamService(teamRepo)

	leaderboardRepo := leaderboard.NewLeaderboardRepository(pool)
	leaderboardService := leaderboard.NewLeaderboardService(leaderboardRepo)

	sessionRepo := session.NewSessionRepository(pool)
	sessionService := session.NewSessionService(sessionRepo)

	fh := frontend.NewFrontendHandler(
		leaderboardService,
		teamService,
		playerService,
		authService,
		sessionService,
		templatesDir,
	)

	r := newBaseRouter(pool)
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	r.Group(func(r chi.Router) {
		r.Use(authmw.OptionalAuthenticateWithSessions(jwtSecret, sessionService))
		r.Use(authmw.CSRFProtect)

		r.Get("/", fh.Leaderboard)
		r.Get("/teams/{id}", fh.TeamDetail)

		r.Get("/login", fh.LoginPage)
		r.Post("/login", fh.LoginSubmit)
		r.Get("/register", fh.RegisterPage)
		r.Post("/register", fh.RegisterSubmit)
		r.Post("/logout", fh.Logout)

		r.With(authmw.AuthenticateWithSessions(jwtSecret, sessionService)).
			Get("/profile", fh.Profile)

		r.With(
			authmw.AuthenticateWithSessions(jwtSecret, sessionService),
			authmw.RequireRole("moderator", "admin"),
		).Get("/mod", fh.ModeratorPanel)
		r.With(
			authmw.AuthenticateWithSessions(jwtSecret, sessionService),
			authmw.RequireRole("moderator", "admin"),
		).Post("/mod/award-points", fh.AwardPoints)
		r.With(
			authmw.AuthenticateWithSessions(jwtSecret, sessionService),
			authmw.RequireRole("moderator", "admin"),
		).Post("/mod/grant-kredits", fh.GrantKredits)
	})
	return r
}
