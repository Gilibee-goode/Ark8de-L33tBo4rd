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
	"context"    // Context for cancellation and deadlines — used for graceful shutdown and DB ping
	"fmt"        // string formatting — used to build error messages and the listen address
	"log/slog"   // structured logging (Go 1.21+) — outputs key=value pairs, easy to parse in production
	"net/http"   // the Go standard HTTP server and handler interfaces
	"os"         // access to environment variables and OS signals
	"os/signal"  // lets us listen for OS signals like Ctrl+C (SIGINT) or Docker's SIGTERM
	"syscall"    // provides the signal constants SIGINT and SIGTERM
	"time"       // used for server timeouts and the graceful shutdown deadline

	// --- Third-party ---
	"github.com/joho/godotenv" // loads .env files into environment variables at startup

	// --- Internal packages ---
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/app" // shared router builder — used by both server and tests
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/db"  // database connection pool wrapper
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
	// Step 5: Build the HTTP router with all routes and handlers
	// -----------------------------------------------------------------
	// app.BuildRouter is a shared function that wires up the entire
	// dependency graph (repos → services → handlers) and registers all
	// routes. We extracted it into internal/app/ so that both the server
	// and the integration test suite use the exact same router config.
	//
	// "templates" is the path to the templates/ folder, relative to the
	// monolith/ directory (which is where the server runs from).
	r := app.BuildRouter(pool, jwtSecret, "templates")

	// -----------------------------------------------------------------
	// Step 6: Start the HTTP server in a goroutine
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
	// Step 7: Wait for shutdown signal, then gracefully shut down
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
