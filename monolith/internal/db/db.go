// Package db — Database connection layer
//
// This file is responsible for ONE thing only: establishing and returning
// a connection pool to PostgreSQL.
//
// It is NOT allowed to:
//   - Contain SQL queries (those belong in repository.go files)
//   - Contain business logic (that belongs in service.go files)
//   - Know anything about HTTP (that belongs in handler.go files)
//
// Any package in the monolith that needs a DB connection imports this
// package and calls Connect() once at startup, then passes the pool around.
package db

import (
	// --- Standard library ---
	"context" // provides Context for cancellation and deadlines — passed into all DB operations
	"fmt"     // string formatting and error wrapping with fmt.Errorf
	"log/slog" // structured logging — Go 1.21+ stdlib, outputs key=value pairs instead of plain strings

	// --- Third-party ---
	"github.com/jackc/pgx/v5/pgxpool" // PostgreSQL connection pool — pgx is faster than database/sql
	// and has better Postgres-specific features (UUIDs, arrays, JSONB, etc.)
)

// Connect creates a new PostgreSQL connection pool and verifies it with a Ping.
//
// A connection pool is a set of pre-opened database connections that are
// reused across requests. Opening a new TCP connection for every query would
// be slow (~ms overhead); a pool keeps connections alive and hands them out
// to callers, collecting them back when done.
//
// Parameters:
//   - ctx: a Context for cancellation — if the parent is cancelled (e.g. app
//     shutting down), the connection attempt is also cancelled
//   - dbURL: a PostgreSQL connection string, e.g.
//     "postgres://user:pass@localhost:5432/dbname?sslmode=disable"
//
// Returns:
//   - *pgxpool.Pool: the live connection pool, ready to use
//   - error: non-nil if the pool could not be created or the DB is unreachable
func Connect(ctx context.Context, dbURL string) (*pgxpool.Pool, error) {
	// pgxpool.New parses the connection string and creates the pool.
	// It does NOT open connections immediately — they are opened lazily
	// on first use, or eagerly after we call Ping below.
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		// fmt.Errorf with %w "wraps" the original error so callers can
		// inspect it with errors.Is() or errors.As() while still seeing
		// the human-readable context we added here.
		return nil, fmt.Errorf("db.Connect: failed to create connection pool: %w", err)
	}

	// Ping sends a lightweight query to verify the database is actually
	// reachable. pgxpool.New only parses the URL; it doesn't test the network.
	// Without Ping, a misconfigured DATABASE_URL would only fail on the first
	// real query, which is harder to diagnose.
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("db.Connect: database ping failed — is PostgreSQL running and is DATABASE_URL correct? %w", err)
	}

	slog.Info("database connection pool established", "max_conns", pool.Config().MaxConns)

	return pool, nil
}
