// Package app — Application wiring
//
// This file provides a shared function to build the HTTP router with all routes
// and middleware. It is used by both the main server (cmd/server/main.go) and
// the integration test suite (tests/setup_test.go).
//
// WHY EXTRACT THIS?
//   The main.go file previously built the router inline. By extracting the
//   router construction into a standalone function, we can build the exact same
//   router in tests — ensuring our tests exercise the real middleware chain,
//   route patterns, and handler wiring, not a hand-assembled imitation.
//
//   This is called the "shared router" pattern and is a common Go testing
//   strategy for integration tests.
package app

import (
	// --- Standard library ---
	"context"  // provides Context for cancellation and deadlines — used for health check DB ping
	"fmt"      // string formatting — used for healthz response text
	"log/slog" // structured logging — used by healthz handler to log DB ping failures
	"net/http" // standard HTTP types — http.Handler, http.ResponseWriter, http.Request
	"time"     // used for healthz DB ping timeout

	// --- Third-party ---
	"github.com/go-chi/chi/v5"           // HTTP router — idiomatic Go, uses standard net/http interfaces
	"github.com/go-chi/chi/v5/middleware" // chi's built-in middleware: request IDs, logging, panic recovery
	"github.com/jackc/pgx/v5/pgxpool"    // PostgreSQL connection pool — needed for healthz DB ping

	// --- Internal packages ---
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/auth"        // auth package: register, login, JWT
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/frontend"    // server-rendered HTML pages
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/leaderboard" // read-only leaderboard API
	authmw "github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/middleware" // JWT + session middleware — aliased to avoid collision with chi's middleware
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/player"      // player profile, skills, gear, kredits
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/session"     // server-side sessions for cookie-based auth
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/team"        // team management and join requests
)

// BuildRouter constructs the complete chi router with all routes, middleware,
// and handlers wired up. It takes the database pool, JWT secret, and path to
// the templates directory.
//
// This function is called by:
//   - cmd/server/main.go — to start the production server
//   - tests/setup_test.go — to create a test server with the exact same routing
//
// It returns a chi.Router which implements http.Handler, so it can be passed
// directly to http.Server or httptest.NewServer.
func BuildRouter(pool *pgxpool.Pool, jwtSecret string, templatesDir string) chi.Router {
	// -----------------------------------------------------------------
	// Build the dependency graph (repos → services → handlers)
	// -----------------------------------------------------------------
	// We wire dependencies manually — no DI framework needed for this size.
	// The pattern is always: repository needs the DB pool; service needs the
	// repository; handler needs the service. Build from the bottom up.

	// Auth
	authRepo := auth.NewPlayerRepository(pool)
	authService := auth.NewAuthService(authRepo, jwtSecret)
	authHandler := auth.NewAuthHandler(authService)

	// Player
	playerRepo := player.NewPlayerRepository(pool)
	playerService := player.NewPlayerService(playerRepo)
	playerHandler := player.NewPlayerHandler(playerService)

	// Team
	teamRepo := team.NewTeamRepository(pool)
	teamService := team.NewTeamService(teamRepo)
	teamHandler := team.NewTeamHandler(teamService)

	// Leaderboard
	leaderboardRepo := leaderboard.NewLeaderboardRepository(pool)
	leaderboardService := leaderboard.NewLeaderboardService(leaderboardRepo)
	leaderboardHandler := leaderboard.NewLeaderboardHandler(leaderboardService)

	// Session — server-side sessions for browser-based cookie auth.
	// The session service is used by the middleware (to look up sessions on each request)
	// and by the frontend handler (to create/destroy sessions on login/logout).
	sessionRepo := session.NewSessionRepository(pool)
	sessionService := session.NewSessionService(sessionRepo)

	// Frontend (server-rendered HTML pages)
	// Now receives authService and sessionService for login/register/logout forms.
	frontendHandler := frontend.NewFrontendHandler(
		leaderboardService,
		teamService,
		playerService,
		authService,
		sessionService,
		templatesDir,
	)

	// -----------------------------------------------------------------
	// Create router and register global middleware
	// -----------------------------------------------------------------
	r := chi.NewRouter()

	// Global middleware runs before every request handler, in the order registered.
	r.Use(middleware.RequestID) // adds a unique X-Request-Id header to every request
	r.Use(middleware.Logger)    // logs method, path, status code, and duration
	r.Use(middleware.Recoverer) // catches panics and returns 500 instead of crashing

	// ---- Static files (CSS, images, etc.) ----
	// http.FileServer serves files from the local filesystem.
	// http.StripPrefix removes the "/static" prefix before looking up the file.
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// ---- Health check ----
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		pingCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
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

	// ======================================================================
	// Frontend HTML pages (server-rendered — returns HTML, not JSON)
	// ======================================================================
	// All frontend routes use:
	//   - OptionalAuthenticateWithSessions: reads session cookie if present,
	//     injects player identity into context for the navbar, but does NOT
	//     return 401 if no session is found (public pages still render).
	//   - CSRFProtect: ensures a csrf_token cookie exists on GET and validates
	//     the token on POST/PUT/DELETE (double-submit cookie pattern).
	r.Group(func(r chi.Router) {
		r.Use(authmw.OptionalAuthenticateWithSessions(jwtSecret, sessionService))
		r.Use(authmw.CSRFProtect)

		// Public pages — anyone can view
		r.Get("/", frontendHandler.Leaderboard)
		r.Get("/teams/{id}", frontendHandler.TeamDetail)

		// Auth pages — login/register forms (redirect if already logged in)
		r.Get("/login", frontendHandler.LoginPage)
		r.Post("/login", frontendHandler.LoginSubmit)
		r.Get("/register", frontendHandler.RegisterPage)
		r.Post("/register", frontendHandler.RegisterSubmit)
		r.Post("/logout", frontendHandler.Logout)

		// Authenticated pages — require a valid session or JWT
		r.With(authmw.AuthenticateWithSessions(jwtSecret, sessionService)).
			Get("/profile", frontendHandler.Profile)

		// Moderator pages — require moderator or admin role
		r.With(
			authmw.AuthenticateWithSessions(jwtSecret, sessionService),
			authmw.RequireRole("moderator", "admin"),
		).Get("/mod", frontendHandler.ModeratorPanel)

		r.With(
			authmw.AuthenticateWithSessions(jwtSecret, sessionService),
			authmw.RequireRole("moderator", "admin"),
		).Post("/mod/award-points", frontendHandler.AwardPoints)

		r.With(
			authmw.AuthenticateWithSessions(jwtSecret, sessionService),
			authmw.RequireRole("moderator", "admin"),
		).Post("/mod/grant-kredits", frontendHandler.GrantKredits)
	})

	// ======================================================================
	// Auth API (returns JSON — uses Bearer JWT, no CSRF needed)
	// ======================================================================
	r.Post("/auth/register", authHandler.Register)
	r.Post("/auth/login", authHandler.Login)
	r.With(authmw.Authenticate(jwtSecret)).Get("/auth/me", authHandler.Me)

	// ======================================================================
	// Leaderboard API (returns JSON)
	// ======================================================================
	r.Get("/api/leaderboard", leaderboardHandler.GetLeaderboard)
	r.Get("/api/leaderboard/teams/{id}", leaderboardHandler.GetTeamCard)

	// ======================================================================
	// Team API (returns JSON)
	// ======================================================================
	r.Route("/api/teams", func(r chi.Router) {
		// Public
		r.Get("/", teamHandler.ListTeams)
		r.Get("/{id}", teamHandler.GetTeam)

		// Authenticated
		r.With(authmw.Authenticate(jwtSecret)).Post("/", teamHandler.CreateTeam)
		r.With(authmw.Authenticate(jwtSecret)).Put("/{id}", teamHandler.UpdateTeam)
		r.With(authmw.Authenticate(jwtSecret)).Delete("/{id}", teamHandler.DeleteTeam)
		r.With(authmw.Authenticate(jwtSecret)).Put("/{id}/lock", teamHandler.ToggleLock)
		r.With(authmw.Authenticate(jwtSecret)).Delete("/{id}/members/{pid}", teamHandler.RemoveMember)
		r.With(authmw.Authenticate(jwtSecret)).Post("/{id}/join-requests", teamHandler.SendJoinRequest)
		r.With(authmw.Authenticate(jwtSecret)).Get("/{id}/join-requests", teamHandler.GetJoinRequests)
		r.With(authmw.Authenticate(jwtSecret)).Put("/{id}/join-requests/{rid}", teamHandler.ResolveJoinRequest)
		r.With(authmw.Authenticate(jwtSecret)).Put("/{id}/logo", teamHandler.UploadLogo)

		// Moderator/admin
		r.With(
			authmw.Authenticate(jwtSecret),
			authmw.RequireRole("moderator", "admin"),
		).Put("/{id}/points", teamHandler.AddArkadePoints)
		r.With(
			authmw.Authenticate(jwtSecret),
			authmw.RequireRole("moderator", "admin"),
		).Get("/{id}/points/history", teamHandler.GetArkadePointHistory)
	})

	// ======================================================================
	// Player API (returns JSON)
	// ======================================================================
	r.Get("/api/skills", playerHandler.ListSkills)

	r.Route("/api/players", func(r chi.Router) {
		// Public
		r.Get("/{id}", playerHandler.GetPublicProfile)

		// Authenticated — player's own operations
		r.With(authmw.Authenticate(jwtSecret)).Put("/me/class", playerHandler.SetClass)
		r.With(authmw.Authenticate(jwtSecret)).Get("/me/stats", playerHandler.GetStats)
		r.With(authmw.Authenticate(jwtSecret)).Get("/me/skills", playerHandler.GetSkills)
		r.With(authmw.Authenticate(jwtSecret)).Put("/me/skills", playerHandler.SetSkills)
		r.With(authmw.Authenticate(jwtSecret)).Get("/me/gear", playerHandler.GetGear)
		r.With(authmw.Authenticate(jwtSecret)).Put("/me/gear", playerHandler.SetGear)
		r.With(authmw.Authenticate(jwtSecret)).Get("/me/kredits", playerHandler.GetKredits)
		r.With(authmw.Authenticate(jwtSecret)).Post("/me/kredits/transfer", playerHandler.TransferKredits)

		// Moderator/admin
		r.With(
			authmw.Authenticate(jwtSecret),
			authmw.RequireRole("moderator", "admin"),
		).Post("/{id}/kredits", playerHandler.GrantKredits)
	})

	return r
}
