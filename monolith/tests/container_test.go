// Package tests — Testcontainers helper for self-contained integration tests
//
// This file provides startPostgresContainer(), which spins up a fresh
// PostgreSQL container using testcontainers-go, then runs all database
// migrations against it. The result is a clean, isolated database that
// exists only for the duration of the test run.
//
// WHAT IS TESTCONTAINERS?
//   testcontainers-go is a Go library that creates real Docker containers
//   from inside your test code. Instead of depending on a pre-running
//   database (like docker-compose dev), the test suite launches its own
//   PostgreSQL container, uses it, and destroys it when done.
//
//   This means:
//     - No manual setup before running tests (no `task dev` needed)
//     - Each test run gets a clean database (no leftover data from dev)
//     - Works in CI (GitHub Actions) without configuring external services
//     - Tests are truly self-contained and reproducible
//
// HOW IT WORKS:
//   1. testcontainers pulls the postgres:16-alpine Docker image (if not cached)
//   2. It starts a container with a random host port mapped to 5432
//   3. It waits until PostgreSQL is ready to accept connections
//   4. We run golang-migrate against the fresh database (creates all tables)
//   5. Tests run against this database
//   6. The container is destroyed when tests finish (via the cleanup function)
//
// PREREQUISITES:
//   Docker must be running on the machine. testcontainers uses the Docker
//   daemon to create and manage containers.
package tests

import (
	// --- Standard library ---
	"context" // context for container operations — testcontainers uses contexts for lifecycle management
	"errors"  // errors.Is to check for migrate.ErrNoChange (no pending migrations)
	"fmt"     // string formatting for error messages
	"time"    // time.Second for container startup timeout configuration

	// --- Third-party ---
	"github.com/golang-migrate/migrate/v4" // programmatic migration runner — same library as cmd/migrate
	// These blank imports register drivers with golang-migrate.
	// The underscore _ means "import for side effects only" — the package's init()
	// function registers itself as a driver, but we never call its functions directly.
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // registers "postgres://" as a valid database URL scheme
	_ "github.com/golang-migrate/migrate/v4/source/file"       // registers "file://" as a valid source for reading .sql files
	"github.com/testcontainers/testcontainers-go"               // core library for managing Docker containers in tests
	"github.com/testcontainers/testcontainers-go/modules/postgres" // pre-configured PostgreSQL container module
	"github.com/testcontainers/testcontainers-go/wait"          // wait strategies — how to know when the container is ready
)

// startPostgresContainer launches a disposable PostgreSQL container and runs
// all database migrations against it.
//
// It returns:
//   - connStr: a PostgreSQL connection string (e.g. "postgres://test:test@localhost:55432/ark8de_test?sslmode=disable")
//   - cleanup: a function that stops and removes the container — call this in TestMain after tests finish
//   - err:     non-nil if anything goes wrong (Docker not running, migrations fail, etc.)
//
// The caller is responsible for calling cleanup() when done. Typical usage:
//
//	connStr, cleanup, err := startPostgresContainer(ctx)
//	if err != nil { log.Fatal(err) }
//	defer cleanup()
func startPostgresContainer(ctx context.Context) (connStr string, cleanup func(), err error) {
	// ---- Step 1: Start the PostgreSQL container ----
	//
	// postgres.Run is a convenience function from the testcontainers postgres module.
	// It creates a PostgreSQL container with the specified image and configuration.
	//
	// We use postgres:16-alpine because:
	//   - PostgreSQL 16 matches our production version
	//   - Alpine is a tiny Linux image (~5MB), so the container starts faster
	//
	// The wait strategy tells testcontainers how to know when PostgreSQL is ready.
	// PostgreSQL logs "database system is ready to accept connections" twice during
	// startup (once for the initial setup, once for the restart). We wait for the
	// second occurrence to ensure the database is fully ready.
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("ark8de_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return "", nil, fmt.Errorf("startPostgresContainer: failed to start container: %w", err)
	}

	// Build the cleanup function that the caller will defer.
	// testcontainers.Terminate stops the container and removes it from Docker.
	cleanup = func() {
		_ = pgContainer.Terminate(ctx)
	}

	// ---- Step 2: Get the connection string ----
	//
	// ConnectionString returns a DSN like "postgres://test:test@localhost:55432/ark8de_test?"
	// The port is randomly assigned by Docker — testcontainers handles the mapping.
	// We append "sslmode=disable" because local containers don't use SSL.
	connStr, err = pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("startPostgresContainer: failed to get connection string: %w", err)
	}

	// ---- Step 3: Run database migrations ----
	//
	// golang-migrate reads .sql files from the file system and applies them in order.
	// Our migrations live at <repo-root>/db/migrations/.
	//
	// Since tests run from monolith/tests/ and we chdir to monolith/ in TestMain,
	// the relative path "../db/migrations" points to <repo-root>/db/migrations/.
	//
	// migrate.New connects to the database and discovers which migrations need to run.
	// m.Up() applies all pending migrations (creates tables, indexes, seeds gear_types, etc.).
	m, err := migrate.New("file://../db/migrations", connStr)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("startPostgresContainer: failed to create migrate instance: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		cleanup()
		return "", nil, fmt.Errorf("startPostgresContainer: migrations failed: %w", err)
	}

	return connStr, cleanup, nil
}
