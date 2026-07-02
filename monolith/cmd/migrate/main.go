// Package main — Database migration CLI tool
//
// This is a standalone command-line program (not the HTTP server).
// Usage:
//
//	go run ./cmd/migrate up    — apply all pending migrations
//	go run ./cmd/migrate down  — roll back the most recent migration
//
// WHAT ARE MIGRATIONS?
//
// A database migration is a versioned, ordered SQL script that changes the
// database schema (creates tables, adds columns, creates indexes, etc.).
// Migrations come in pairs:
//   - .up.sql   — applies the change (e.g. CREATE TABLE players ...)
//   - .down.sql — reverses it       (e.g. DROP TABLE players)
//
// golang-migrate tracks which migrations have been applied by storing a
// record in a special `schema_migrations` table it creates automatically.
// Running `migrate up` only applies migrations that haven't been applied yet,
// so it's safe to run repeatedly.
//
// WHY A SEPARATE BINARY?
//
// Migrations are run separately from the server so that:
//   1. The server doesn't need migration-related code in production
//   2. Migrations can be run manually, in CI, or as a Kubernetes Job
//   3. The server can start without running migrations (in testing)
package main

import (
	// --- Standard library ---
	"errors" // for inspecting specific error values from the migrate library
	"fmt"    // for formatting output messages
	"log/slog" // structured logging — same logger pattern as the server
	"os"     // reading environment variables and command-line arguments

	// --- Third-party ---
	"github.com/golang-migrate/migrate/v4"                  // the migration runner — tracks and applies SQL files
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // registers the "postgres" database driver for migrate
	// The blank import `_` imports a package solely for its side effects —
	// in this case, the package's init() function registers itself with migrate
	// as the "postgres" driver. We never call any functions from it directly.
	_ "github.com/golang-migrate/migrate/v4/source/file" // registers the "file://" source driver for reading .sql files from disk
	"github.com/joho/godotenv"                           // loads .env files into environment variables
)

func main() {
	// Load .env if it exists — same as the server, ignore the error if absent.
	if err := godotenv.Load(); err != nil {
		slog.Info("no .env file found, reading config from environment directly")
	}

	// os.Args is a slice of command-line arguments.
	// os.Args[0] is always the program name; os.Args[1] is the first argument.
	// We require exactly one argument: "up" or "down".
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run ./cmd/migrate [up|down]")
		os.Exit(1)
	}

	// The subcommand is the first argument after the program name.
	command := os.Args[1]
	if command != "up" && command != "down" {
		fmt.Printf("Unknown command %q. Use 'up' or 'down'.\n", command)
		os.Exit(1)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		slog.Error("DATABASE_URL environment variable is required but not set")
		os.Exit(1)
	}

	// Run the migrations and handle errors.
	if err := runMigrations(dbURL, command); err != nil {
		slog.Error("migration failed", "command", command, "error", err)
		os.Exit(1)
	}
}

// runMigrations creates a migrate instance pointed at our SQL files and
// runs the requested command ("up" or "down").
//
// Parameters:
//   - dbURL: PostgreSQL connection string
//   - command: "up" to apply all pending migrations, "down" to roll back one
//
// Returns an error if the migration fails, or nil on success.
func runMigrations(dbURL, command string) error {
	// The source URL tells golang-migrate where to find the .sql files.
	// "file://" means read from the filesystem at the given path.
	// The path is relative to where the binary is run from, which for
	// `go run ./cmd/migrate` from the monolith/ directory is monolith/.
	// Our migrations live at <repo-root>/db/migrations/, which is one
	// level up from monolith/.
	// MIGRATIONS_PATH lets containers point at a baked-in copy of the SQL
	// files (e.g. /app/migrations); the default suits `go run` from monolith/.
	migrationsPath := os.Getenv("MIGRATIONS_PATH")
	if migrationsPath == "" {
		migrationsPath = "../db/migrations"
	}
	sourceURL := "file://" + migrationsPath

	// migrate.New creates a migration runner.
	// It reads the source directory, discovers all *.up.sql and *.down.sql
	// files, and connects to the database to check what's already been applied.
	m, err := migrate.New(sourceURL, dbURL)
	if err != nil {
		return fmt.Errorf("runMigrations: failed to create migrate instance: %w", err)
	}
	// defer m.Close() closes the database connection and source reader
	// when this function returns — good practice to avoid resource leaks.
	defer m.Close()

	switch command {
	case "up":
		// m.Up() applies all pending migrations in numerical order.
		// It is idempotent — already-applied migrations are skipped.
		slog.Info("running all pending migrations...")
		if err := m.Up(); err != nil {
			// migrate.ErrNoChange is returned when there are no pending migrations.
			// This is NOT an error — it means we're already up-to-date.
			if errors.Is(err, migrate.ErrNoChange) {
				slog.Info("no pending migrations — database is already up to date")
				return nil
			}
			return fmt.Errorf("runMigrations: migrate up failed: %w", err)
		}
		slog.Info("all migrations applied successfully")

	case "down":
		// m.Steps(-1) rolls back exactly ONE migration (the most recent).
		// We use Steps(-1) rather than m.Down() because m.Down() rolls back
		// ALL migrations, which would delete all your data in development.
		// Rolling back one at a time is safer for learning and debugging.
		slog.Info("rolling back one migration...")
		if err := m.Steps(-1); err != nil {
			if errors.Is(err, migrate.ErrNoChange) {
				slog.Info("no migrations to roll back")
				return nil
			}
			return fmt.Errorf("runMigrations: migrate down failed: %w", err)
		}
		slog.Info("migration rolled back successfully")
	}

	return nil
}
