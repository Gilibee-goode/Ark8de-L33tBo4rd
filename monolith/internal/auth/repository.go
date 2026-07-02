// Package auth — Database Repository layer
//
// This file is ONLY responsible for:
//   - Executing SQL queries against the `players` table
//   - Scanning database rows into Player structs
//   - Returning domain types (Player) or errors to the service layer
//
// It must NOT contain:
//   - Business logic (validation, password hashing) → that belongs in service.go
//   - HTTP code (request parsing, response writing) → that belongs in handler.go
package auth

import (
	// --- Standard library ---
	"context" // provides Context — passed into every DB call so the query can be cancelled
	// if the HTTP request is cancelled by the client before the query finishes
	"fmt" // for wrapping errors with context: fmt.Errorf("where it failed: %w", err)

	// --- Third-party ---
	// pgx is the PostgreSQL driver for Go. pgx.ErrNoRows is a sentinel error
	// value the driver returns when a query finds no matching rows.
	"github.com/jackc/pgx/v5"
	// pgxpool manages a pool of reusable database connections.
	// Opening a new TCP connection per query is slow (~ms); a pool keeps
	// connections alive and hands them out to goroutines on demand.
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile-time check: *PlayerRepository must satisfy the Repository interface
// defined in service.go. If a method is missing or has the wrong signature,
// this line causes a compile error — catching mistakes immediately.
//
// The underscore _ means we don't need the variable itself — we only want
// the compiler to verify the type assertion. (*PlayerRepository)(nil) creates
// a nil pointer of type *PlayerRepository, which is enough for the check.
var _ Repository = (*PlayerRepository)(nil)

// PlayerRepository is the database access layer for the auth package.
// It is the only part of this codebase allowed to execute SQL queries
// that touch the `players` table.
//
// In Go, we group related methods onto a struct using a receiver pattern.
// This is similar to a class in other languages, but simpler — there is
// no inheritance, only composition.
type PlayerRepository struct {
	// db is the connection pool. It is shared across all requests — we never
	// open a new connection per request. pgxpool handles the concurrency for us.
	db *pgxpool.Pool
}

// NewPlayerRepository is the constructor for PlayerRepository.
// Go doesn't have constructors like Java/C# — instead, by convention we
// write a plain function named New<Type> that creates and returns the struct.
func NewPlayerRepository(db *pgxpool.Pool) *PlayerRepository {
	return &PlayerRepository{db: db}
}

// playerColumns is the list of columns we SELECT in every player query.
// Defined once here so all queries stay consistent and there's only one
// place to update when the schema changes.
//
// id::text — PostgreSQL stores UUIDs as a 16-byte binary type. The ::text
// cast converts it to a human-readable string like "550e8400-e29b-41d4-a716-446655440000"
// so Go's string type can receive it directly without special UUID handling.
const playerColumns = `
	id::text,
	username,
	email,
	password_hash,
	role,
	profile_photo_url,
	class_role,
	skill_points_total,
	kredits,
	created_at,
	updated_at`

// scanPlayer reads one row of player columns into a Player struct.
// This helper is called by all three query methods to avoid repeating
// the Scan call with its 11 arguments.
//
// IMPORTANT: The order of arguments in row.Scan() MUST exactly match
// the order of columns in playerColumns above. A mismatch silently
// puts the wrong data in the wrong fields, causing hard-to-debug bugs.
//
// pgx.Row is the result of QueryRow — it holds exactly one row (or an error).
func scanPlayer(row pgx.Row) (*Player, error) {
	var p Player
	// row.Scan reads each column value from the result into the pointer targets,
	// left to right, matching the SELECT column order.
	// Nullable columns (profile_photo_url, class_role) scan into *string —
	// a NULL in the DB becomes a nil pointer in Go.
	err := row.Scan(
		&p.ID,               // id::text
		&p.Username,         // username
		&p.Email,            // email
		&p.PasswordHash,     // password_hash
		&p.Role,             // role
		&p.ProfilePhotoURL,  // profile_photo_url (nullable → *string)
		&p.ClassRole,        // class_role        (nullable → *string)
		&p.SkillPointsTotal, // skill_points_total
		&p.Kredits,          // kredits
		&p.CreatedAt,        // created_at
		&p.UpdatedAt,        // updated_at
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// CreatePlayer inserts a new player into the database and returns the full
// created row, including database-generated values like id, role (default "player"),
// skill_points_total (default 20), and the timestamps.
//
// The RETURNING clause in the SQL lets us retrieve the inserted row in a single
// round-trip, rather than doing INSERT then a separate SELECT.
//
// Parameters:
//   - ctx: cancellation context — if the HTTP request is cancelled mid-query, this stops the DB call
//   - username: the player's display name (already validated by the service)
//   - email: the player's login email (already validated, already lowercased)
//   - passwordHash: the bcrypt hash (the plain password is NEVER passed here)
func (r *PlayerRepository) CreatePlayer(ctx context.Context, username, email, passwordHash string) (*Player, error) {
	// $1, $2, $3 are positional placeholders. pgx substitutes the actual values
	// at the DB driver level — this is parameterised SQL, which prevents SQL injection.
	// Never interpolate user input directly into a query string.
	query := `
		INSERT INTO players (username, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING` + playerColumns

	row := r.db.QueryRow(ctx, query, username, email, passwordHash)

	player, err := scanPlayer(row)
	if err != nil {
		// We wrap the error with context so that if this bubbles up to a log,
		// the reader immediately knows where in the code it happened.
		// %w (not %s or %v) wraps the error so callers can use errors.Is() / errors.As()
		// to inspect the underlying error type (e.g. *pgconn.PgError for constraint violations).
		return nil, fmt.Errorf("PlayerRepository.CreatePlayer: %w", err)
	}
	return player, nil
}

// GetByEmail fetches a single player row by email address.
// Called during login to look up the account before verifying the password.
//
// Returns:
//   - (*Player, nil)            if the player was found
//   - (nil, pgx.ErrNoRows)     if no player has that email
//   - (nil, wrapped error)      for any other database failure
func (r *PlayerRepository) GetByEmail(ctx context.Context, email string) (*Player, error) {
	query := `SELECT` + playerColumns + ` FROM players WHERE email = $1`

	row := r.db.QueryRow(ctx, query, email)
	player, err := scanPlayer(row)
	if err != nil {
		// Pass pgx.ErrNoRows through unwrapped so the service layer can detect
		// "no account found" vs "database error" using errors.Is(err, pgx.ErrNoRows).
		// Wrapping it would hide it behind a fmt.Errorf and break that check.
		if err == pgx.ErrNoRows {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("PlayerRepository.GetByEmail: %w", err)
	}
	return player, nil
}

// GetByID fetches a single player row by their UUID.
// Called by GET /auth/me after the JWT middleware has verified the token
// and extracted the player ID from the claims.
//
// The $1::uuid cast tells PostgreSQL to interpret the plain string argument
// as a UUID for the comparison. Without it, PostgreSQL would try to compare
// a text value to a UUID column and might refuse or produce wrong results.
func (r *PlayerRepository) GetByID(ctx context.Context, id string) (*Player, error) {
	query := `SELECT` + playerColumns + ` FROM players WHERE id = $1::uuid`

	row := r.db.QueryRow(ctx, query, id)
	player, err := scanPlayer(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("PlayerRepository.GetByID: %w", err)
	}
	return player, nil
}
