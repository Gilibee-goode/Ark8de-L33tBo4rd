// Package main — Entry point for the Ark8de monolith HTTP server
//
// This file starts the application:
//  1. Loads configuration from the environment (via .env)
//  2. Establishes the database connection pool
//  3. Builds the dependency graph (repos → services → handlers)
//  4. Registers all HTTP routes on the chi router
//  5. Starts the server in a goroutine
//  6. Waits for a shutdown signal (Ctrl+C or SIGTERM from Docker)
//  7. Gracefully drains in-flight requests before exiting
//
// In Go, `package main` is special — it is the entry point of an executable.
// A Go program starts execution in the `main()` function of `package main`.
package main

import (
	// --- Standard library ---
	"context"       // Context for cancellation and deadlines — used for graceful shutdown and DB ping
	"fmt"           // string formatting — used to build error messages and the listen address
	"log/slog"      // structured logging (Go 1.21+) — outputs key=value pairs, easy to parse in production
	"net/http"      // the Go standard HTTP server and handler interfaces
	"os"            // access to environment variables and OS signals
	"os/signal"     // lets us listen for OS signals like Ctrl+C (SIGINT) or Docker's SIGTERM
	"syscall"       // provides the signal constants SIGINT and SIGTERM
	"time"          // used for server timeouts and the graceful shutdown deadline

	// --- Third-party ---
	"github.com/go-chi/chi/v5"            // HTTP router — idiomatic Go, uses standard net/http interfaces
	"github.com/go-chi/chi/v5/middleware" // chi's built-in middleware: request IDs, logging, panic recovery
	"github.com/joho/godotenv"            // loads .env files into environment variables at startup

	// --- Internal packages ---
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/auth"        // auth package: register, login, JWT
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/db"           // database connection pool wrapper
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/frontend"     // server-rendered HTML pages
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/leaderboard"  // read-only leaderboard API
	authmw "github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/middleware" // JWT middleware — aliased to avoid collision with chi's middleware package
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/player"       // player profile, skills, gear, kredits
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/team"         // team management and join requests
)

func main() {
	// -----------------------------------------------------------------
	// Step 1: Load environment variables from .env file
	// -----------------------------------------------------------------
	// godotenv.Load() reads the .env file and sets each KEY=VALUE pair
	// as an environment variable for this process. If .env doesn't exist
	// (e.g. in a Docker container where vars are injected directly),
	// we silently continue — the app will still read vars from the real env.
	if err := godotenv.Load(); err != nil {
		slog.Info("no .env file found, reading config from environment directly")
	}

	// -----------------------------------------------------------------
	// Step 2: Set up structured logging
	// -----------------------------------------------------------------
	// slog is Go's built-in structured logging library (added in Go 1.21).
	// We configure it to write JSON-formatted logs to stdout.
	// JSON logs can be ingested by log aggregators (Loki, Datadog, etc.)
	// without extra parsing configuration.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo, // log Info and above (not Debug)
	}))
	// SetDefault makes this the logger used by slog.Info(), slog.Error(), etc.
	slog.SetDefault(logger)

	// -----------------------------------------------------------------
	// Step 3: Read required configuration from environment
	// -----------------------------------------------------------------
	// os.Getenv reads a single environment variable.
	// It returns "" (empty string) if the variable is not set.
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		slog.Error("DATABASE_URL environment variable is required but not set")
		os.Exit(1) // non-zero exit code signals failure to Docker/systemd
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // standard Go web server port
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		slog.Error("JWT_SECRET environment variable is required but not set")
		os.Exit(1)
	}

	// -----------------------------------------------------------------
	// Step 4: Connect to the database
	// -----------------------------------------------------------------
	// context.Background() creates a root context with no deadline or
	// cancellation. We use it here for the initial connection because
	// we want it to succeed (or fail permanently) regardless of requests.
	ctx := context.Background()

	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	// defer schedules pool.Close() to run when main() returns.
	// In Go, `defer` is a keyword that pushes a function call onto a stack;
	// deferred calls run in LIFO order when the surrounding function exits —
	// whether it exits normally or via panic. This guarantees cleanup happens.
	defer pool.Close()

	slog.Info("connected to PostgreSQL")

	// -----------------------------------------------------------------
	// Step 5: Build the dependency graph (repos → services → handlers)
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

	// Frontend (server-rendered HTML pages)
	// "templates" is the path to the templates/ folder, relative to the
	// monolith/ directory (which is where the server runs from).
	frontendHandler := frontend.NewFrontendHandler(
		leaderboardService,
		teamService,
		playerService,
		"templates",
	)

	// -----------------------------------------------------------------
	// Step 6: Build the HTTP router and register routes
	// -----------------------------------------------------------------
	// chi.NewRouter() creates a new router. In Go, a router implements
	// the http.Handler interface — it has a ServeHTTP(w, r) method that
	// the HTTP server calls for every incoming request.
	r := chi.NewRouter()

	// Global middleware runs before every request handler, in the order registered.
	// chi middleware is just a function that wraps http.Handler — idiomatic Go
	// design that doesn't require learning a framework API.
	r.Use(middleware.RequestID) // adds a unique X-Request-Id header to every request
	r.Use(middleware.Logger)    // logs method, path, status code, and duration
	r.Use(middleware.Recoverer) // catches panics and returns 500 instead of crashing

	// ---- Static files (CSS, images, etc.) ----
	// http.FileServer serves files from the local filesystem.
	// http.StripPrefix removes the "/static" prefix before the file server
	// looks up the file — so a request for /static/style.css becomes style.css
	// in the static/ directory.
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// ---- Health check ----
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		// Create a context with a 5-second deadline for the DB ping.
		// If the ping takes longer, the context is cancelled and Ping() returns an error.
		pingCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		// defer cancel() releases resources associated with this context.
		// Always cancel a timeout context — not doing so leaks goroutines.
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

	// GET / — public leaderboard page
	r.Get("/", frontendHandler.Leaderboard)

	// GET /teams/{id} — public team detail page
	r.Get("/teams/{id}", frontendHandler.TeamDetail)

	// GET /profile — authenticated player's own profile page
	// r.With() creates a per-route middleware chain.
	// authmw.Authenticate verifies the JWT before the handler runs.
	r.With(authmw.Authenticate(jwtSecret)).Get("/profile", frontendHandler.Profile)

	// GET /mod — moderator action panel (moderator or admin only)
	// RequireRole must come AFTER Authenticate — it reads the role that
	// Authenticate stored in the context.
	r.With(
		authmw.Authenticate(jwtSecret),
		authmw.RequireRole("moderator", "admin"),
	).Get("/mod", frontendHandler.ModeratorPanel)

	// ======================================================================
	// Auth API  (returns JSON)
	// ======================================================================

	r.Post("/auth/register", authHandler.Register)
	r.Post("/auth/login", authHandler.Login)
	r.With(authmw.Authenticate(jwtSecret)).Get("/auth/me", authHandler.Me)

	// ======================================================================
	// Leaderboard API  (returns JSON)
	// ======================================================================

	// GET /api/leaderboard — all teams in ranked order
	r.Get("/api/leaderboard", leaderboardHandler.GetLeaderboard)
	// GET /api/leaderboard/teams/{id} — one team's leaderboard card
	r.Get("/api/leaderboard/teams/{id}", leaderboardHandler.GetTeamCard)

	// ======================================================================
	// Team API  (returns JSON)
	// ======================================================================

	// r.Route groups routes under a common path prefix.
	// All routes inside share the "/api/teams" prefix automatically.
	r.Route("/api/teams", func(r chi.Router) {
		// Public — no auth required
		r.Get("/", teamHandler.ListTeams)     // GET /api/teams
		r.Get("/{id}", teamHandler.GetTeam)   // GET /api/teams/{id}

		// Authenticated — any logged-in player
		r.With(authmw.Authenticate(jwtSecret)).Post("/", teamHandler.CreateTeam)
		r.With(authmw.Authenticate(jwtSecret)).Put("/{id}", teamHandler.UpdateTeam)
		r.With(authmw.Authenticate(jwtSecret)).Delete("/{id}", teamHandler.DeleteTeam)
		r.With(authmw.Authenticate(jwtSecret)).Put("/{id}/lock", teamHandler.ToggleLock)
		r.With(authmw.Authenticate(jwtSecret)).Delete("/{id}/members/{pid}", teamHandler.RemoveMember)
		r.With(authmw.Authenticate(jwtSecret)).Post("/{id}/join-requests", teamHandler.SendJoinRequest)
		r.With(authmw.Authenticate(jwtSecret)).Get("/{id}/join-requests", teamHandler.GetJoinRequests)
		r.With(authmw.Authenticate(jwtSecret)).Put("/{id}/join-requests/{rid}", teamHandler.ResolveJoinRequest)
		r.With(authmw.Authenticate(jwtSecret)).Put("/{id}/logo", teamHandler.UploadLogo)

		// Moderator/admin only — Arkade point management
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
	// Player API  (returns JSON)
	// ======================================================================

	// GET /api/skills — public list of all skills, optionally filtered by ?class_role=
	r.Get("/api/skills", playerHandler.ListSkills)

	r.Route("/api/players", func(r chi.Router) {
		// Public — anyone can view a player's public profile
		r.Get("/{id}", playerHandler.GetPublicProfile)

		// Authenticated player's own operations
		// NOTE: chi matches static path segments (like "me") before parameterised
		// ones (like {id}), so /me/class routes are always preferred over /{id}/class.
		r.With(authmw.Authenticate(jwtSecret)).Put("/me/class", playerHandler.SetClass)
		r.With(authmw.Authenticate(jwtSecret)).Get("/me/stats", playerHandler.GetStats)
		r.With(authmw.Authenticate(jwtSecret)).Get("/me/skills", playerHandler.GetSkills)
		r.With(authmw.Authenticate(jwtSecret)).Put("/me/skills", playerHandler.SetSkills)
		r.With(authmw.Authenticate(jwtSecret)).Get("/me/gear", playerHandler.GetGear)
		r.With(authmw.Authenticate(jwtSecret)).Put("/me/gear", playerHandler.SetGear)
		r.With(authmw.Authenticate(jwtSecret)).Get("/me/kredits", playerHandler.GetKredits)
		r.With(authmw.Authenticate(jwtSecret)).Post("/me/kredits/transfer", playerHandler.TransferKredits)

		// Moderator/admin — grant Kredits to any player by ID
		r.With(
			authmw.Authenticate(jwtSecret),
			authmw.RequireRole("moderator", "admin"),
		).Post("/{id}/kredits", playerHandler.GrantKredits)
	})

	// -----------------------------------------------------------------
	// Step 7: Start the HTTP server in a goroutine
	// -----------------------------------------------------------------
	// A goroutine is a lightweight thread managed by the Go runtime.
	// `go func() { ... }()` launches the function concurrently — it runs
	// alongside the rest of main() without blocking it.
	// We need the server in a goroutine so the shutdown signal code below
	// can also run. Calling ListenAndServe() directly would block forever.

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", port),
		Handler: r,

		// Timeouts prevent slow clients from holding connections open forever.
		ReadTimeout:  15 * time.Second, // max time to read request headers + body
		WriteTimeout: 15 * time.Second, // max time to write the response
		IdleTimeout:  60 * time.Second, // max time for idle keep-alive connections
	}

	go func() {
		slog.Info("server starting", "addr", srv.Addr)
		// ListenAndServe blocks until an error occurs or the server shuts down.
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// http.ErrServerClosed is expected during graceful shutdown — not an error.
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// -----------------------------------------------------------------
	// Step 8: Wait for shutdown signal, then gracefully shut down
	// -----------------------------------------------------------------
	// make(chan os.Signal, 1) creates a buffered channel of capacity 1.
	// A channel is a typed conduit for passing values between goroutines.
	// The buffer means the OS sender won't block if we're not yet reading.
	quit := make(chan os.Signal, 1)

	// signal.Notify registers `quit` to receive SIGINT (Ctrl+C) and SIGTERM
	// (what Docker/Kubernetes sends when stopping a container).
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// <-quit blocks until a signal is received — this is where main() waits.
	<-quit

	slog.Info("shutdown signal received, draining in-flight requests...")

	// Give in-flight requests up to 30 seconds to finish before we force-quit.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// srv.Shutdown stops accepting new connections and waits for in-flight
	// requests to complete, up to the deadline in shutdownCtx.
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed — forcing exit", "error", err)
		os.Exit(1)
	}

	slog.Info("server shut down cleanly")
}
