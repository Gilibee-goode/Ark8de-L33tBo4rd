// Package app — shared bootstrap helpers for the service binaries.
//
// Every microservice main() does the same dance: load .env, set up JSON
// logging, read config, connect to Postgres (and maybe NATS), start an HTTP
// server, and shut down gracefully on SIGINT/SIGTERM. These helpers keep each
// main() to a few lines.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/db"
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/events"
)

// Bootstrap loads .env (if present) and switches slog to JSON output.
func Bootstrap() {
	if err := godotenv.Load(); err != nil {
		slog.Info("no .env file found, reading config from environment directly")
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
}

// MustEnv returns the value of a required environment variable, exiting the
// process if it is unset.
func MustEnv(name string) string {
	v := os.Getenv(name)
	if v == "" {
		slog.Error("required environment variable not set", "name", name)
		os.Exit(1)
	}
	return v
}

// EnvOr returns the environment variable's value, or def if unset.
func EnvOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// MustConnectDB connects to Postgres using DATABASE_URL, exiting on failure.
func MustConnectDB(ctx context.Context) *pgxpool.Pool {
	pool, err := db.Connect(ctx, MustEnv("DATABASE_URL"))
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to PostgreSQL")
	return pool
}

// MaybeNATS connects to NATS if NATS_URL is set. Returns (publisher, nil)
// when configured, (nil, NoopPublisher) otherwise — so callers can always
// pass the second value to WithEvents.
func MaybeNATS() (*events.NATSPublisher, events.Publisher) {
	url := os.Getenv("NATS_URL")
	if url == "" {
		slog.Info("NATS_URL not set — events disabled (noop publisher)")
		return nil, events.NoopPublisher{}
	}
	pub, err := events.ConnectNATS(url)
	if err != nil {
		slog.Error("failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	return pub, pub
}

// RunServer starts an HTTP server on :port and blocks until SIGINT/SIGTERM,
// then drains in-flight requests for up to 30 seconds.
func RunServer(handler http.Handler, port string) {
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutdown signal received, draining in-flight requests...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed — forcing exit", "error", err)
		os.Exit(1)
	}
	slog.Info("server shut down cleanly")
}
