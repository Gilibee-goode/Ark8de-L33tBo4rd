// Package auth — Business Logic / Service layer
//
// This file is ONLY responsible for:
//   - Validating inputs (username length, password strength, email format)
//   - Enforcing business rules (no duplicate accounts, credentials must match)
//   - Orchestrating calls between the repository (DB) and JWT generation
//
// It must NOT contain:
//   - SQL queries → that belongs in repository.go
//   - HTTP code (request/response) → that belongs in handler.go
package auth

import (
	// --- Standard library ---
	"context" // for passing context down to repository calls (cancellation / deadlines)
	"errors"  // for defining sentinel errors and checking them with errors.Is / errors.As
	"fmt"     // for wrapping errors with context: fmt.Errorf("location: %w", err)
	"strings" // for trimming whitespace and lowercasing emails before storing
	"time"    // for computing JWT expiry time (time.Now().Add(24 * time.Hour))
	"unicode" // for checking character categories in password/username validation

	// --- Third-party ---
	// golang-jwt/jwt is the library we use to create and verify JWTs.
	// A JWT (JSON Web Token) is a signed, self-contained token that proves
	// who the player is without hitting the database on every request.
	"github.com/golang-jwt/jwt/v5"

	// pgx is the PostgreSQL driver. We import it here only for pgx.ErrNoRows —
	// a sentinel error the repository returns when a query finds nothing.
	"github.com/jackc/pgx/v5"

	// pgconn is a sub-package of pgx that defines PostgreSQL-specific error types.
	// We use *pgconn.PgError to detect unique constraint violations (duplicate email/username).
	"github.com/jackc/pgx/v5/pgconn"

	// bcrypt is the industry-standard algorithm for hashing passwords.
	// It is deliberately slow (to resist brute-force attacks) and includes
	// a random salt automatically (to prevent rainbow table attacks).
	"golang.org/x/crypto/bcrypt"
)

// Sentinel errors are package-level error values that callers can check with errors.Is().
// Using named errors gives callers a stable API to react to specific failures
// (e.g. show "username taken" message) without parsing error strings, which is fragile.
var (
	// ErrEmailTaken is returned when the registration email is already in use.
	ErrEmailTaken = errors.New("a player with that email already exists")

	// ErrUsernameTaken is returned when the registration username is already in use.
	ErrUsernameTaken = errors.New("that username is already taken")

	// ErrInvalidCredentials is returned for both "email not found" and "wrong password"
	// on login. Deliberately vague — we never reveal which one failed, because telling
	// an attacker "that email doesn't exist" helps them enumerate valid accounts.
	ErrInvalidCredentials = errors.New("invalid email or password")

	// ErrWeakPassword is returned when the password doesn't meet our requirements.
	ErrWeakPassword = errors.New("password must be at least 8 characters and contain at least one uppercase letter, one lowercase letter, and one number")

	// ErrInvalidUsername is returned when the username is too short, too long, or contains
	// characters we don't allow (only letters, digits, and underscores).
	ErrInvalidUsername = errors.New("username must be 3–50 characters and contain only letters, numbers, and underscores")

	// ErrInvalidEmail is returned when the email address is clearly malformed.
	ErrInvalidEmail = errors.New("please provide a valid email address")
)

// Repository defines the data-access methods that AuthService needs.
//
// WHY AN INTERFACE?
//   An interface in Go is a contract — a set of method signatures.
//   Any type that has all of these methods automatically satisfies this interface.
//   Go checks this at compile time, but you never write "implements Repository"
//   like you would in Java — it's implicit (called "structural typing").
//
//   By depending on this interface instead of the concrete *PlayerRepository,
//   the AuthService can work with any implementation:
//     - The real PlayerRepository in production (talks to PostgreSQL)
//     - A lightweight mock in unit tests (returns hardcoded data)
//
//   Go idiom: "accept interfaces, return structs."
//   We define the interface HERE (in the consumer package) rather than in the
//   repository package, because the consumer knows what it needs. This keeps
//   the interface minimal — only the 3 methods this service actually calls.
type Repository interface {
	CreatePlayer(ctx context.Context, username, email, passwordHash string) (*Player, error)
	GetByEmail(ctx context.Context, email string) (*Player, error)
	GetByID(ctx context.Context, id string) (*Player, error)
}

// AuthService contains the business logic for authentication.
// It sits between the HTTP handler (which deals with HTTP) and the repository
// (which deals with the database). This separation makes each layer testable
// independently — we can test business logic without a real DB or HTTP server.
type AuthService struct {
	// repo is the database layer. AuthService calls it to read and write player records.
	// The type is Repository (an interface), not *PlayerRepository (a concrete struct).
	// This allows us to swap in a mock repository during unit tests.
	repo Repository

	// jwtSecret is the private key used to sign JWT tokens.
	// It must be kept secret on the server — anyone with this key can forge valid tokens.
	jwtSecret string
}

// NewAuthService creates and returns an AuthService.
// Called once at startup (in main.go) with the shared repository and JWT secret.
//
// The repo parameter is the Repository interface — in production, a *PlayerRepository
// is passed in and satisfies this interface automatically (Go's structural typing).
// In tests, a mock struct with the same methods is passed instead.
func NewAuthService(repo Repository, jwtSecret string) *AuthService {
	return &AuthService{repo: repo, jwtSecret: jwtSecret}
}

// Claims defines what we store inside a JWT token.
//
// A JWT has three parts: header (algorithm), payload (claims), signature.
// The payload is a JSON object — these are the fields we put in it.
//
// jwt.RegisteredClaims is EMBEDDED in our Claims struct using Go's embedding feature.
// Embedding means Claims inherits all the fields of RegisteredClaims (like ExpiresAt,
// IssuedAt, Issuer) as if they were declared directly on Claims.
// This is Go's composition-over-inheritance model — no subclassing, just field promotion.
type Claims struct {
	PlayerID string `json:"player_id"` // the authenticated player's UUID
	Role     string `json:"role"`      // their permission tier: player, team_owner, moderator, admin
	jwt.RegisteredClaims               // standard JWT fields: ExpiresAt, IssuedAt, Issuer, etc.
}

// Register creates a new player account.
// It validates input, hashes the password, stores the player in the DB,
// and returns a JWT so the player is immediately logged in after registering.
//
// Returns:
//   - *PlayerResponse: the new player's public profile (no password hash)
//   - string: a signed JWT the client should include in future requests
//   - error: a sentinel error (ErrEmailTaken, ErrWeakPassword, etc.) or a wrapped DB error
func (s *AuthService) Register(ctx context.Context, req RegisterRequest) (*PlayerResponse, string, error) {
	// Validate inputs BEFORE touching the database — fail fast and cheap.
	// A DB constraint violation gives a cryptic error; our validation gives
	// a clear, human-readable message.
	if err := validateUsername(req.Username); err != nil {
		return nil, "", err
	}
	if err := validateEmail(req.Email); err != nil {
		return nil, "", err
	}
	if err := validatePassword(req.Password); err != nil {
		return nil, "", err
	}

	// Normalise: trim whitespace and lowercase the email.
	// Prevents duplicate accounts from different capitalisations: "User@Example.com" != "user@example.com"
	username := strings.TrimSpace(req.Username)
	email := strings.ToLower(strings.TrimSpace(req.Email))

	// bcrypt.GenerateFromPassword hashes the plain password.
	// bcrypt is intentionally slow — DefaultCost (10) means ~100ms per hash,
	// making brute-force attacks 10,000× slower than a fast hash like MD5.
	// The output includes the salt and cost factor, so it's self-contained.
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", fmt.Errorf("AuthService.Register: failed to hash password: %w", err)
	}

	player, err := s.repo.CreatePlayer(ctx, username, email, string(passwordHash))
	if err != nil {
		// Inspect the DB error: a unique constraint violation (PostgreSQL error code 23505)
		// means the email or username is already taken.
		if isDuplicateConstraint(err, "players_email_key") {
			return nil, "", ErrEmailTaken
		}
		if isDuplicateConstraint(err, "players_username_key") {
			return nil, "", ErrUsernameTaken
		}
		return nil, "", fmt.Errorf("AuthService.Register: failed to create player: %w", err)
	}

	token, err := s.generateToken(player.ID, player.Role)
	if err != nil {
		return nil, "", fmt.Errorf("AuthService.Register: failed to generate JWT: %w", err)
	}

	return player.ToResponse(), token, nil
}

// Login authenticates a player with their email and password.
// On success, returns their profile and a fresh JWT.
//
// Security note: we return the same ErrInvalidCredentials whether the email
// doesn't exist OR the password is wrong. This prevents an attacker from
// learning which emails are registered by observing different error messages.
func (s *AuthService) Login(ctx context.Context, req LoginRequest) (*PlayerResponse, string, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))

	player, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		// errors.Is checks if the error (or any error it wraps) matches pgx.ErrNoRows.
		// We treat "email not found" as wrong credentials — same message, no leaking.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", fmt.Errorf("AuthService.Login: failed to fetch player: %w", err)
	}

	// bcrypt.CompareHashAndPassword verifies the plain password against the stored hash.
	// Returns nil if they match, bcrypt.ErrMismatchedHashAndPassword if they don't.
	// This comparison is timing-safe — it takes the same time regardless of how
	// different the password is, preventing timing attacks.
	if err := bcrypt.CompareHashAndPassword([]byte(player.PasswordHash), []byte(req.Password)); err != nil {
		return nil, "", ErrInvalidCredentials
	}

	token, err := s.generateToken(player.ID, player.Role)
	if err != nil {
		return nil, "", fmt.Errorf("AuthService.Login: failed to generate JWT: %w", err)
	}

	return player.ToResponse(), token, nil
}

// GetPlayer fetches a player's public profile by their ID.
// Called by GET /auth/me after the middleware has verified the JWT.
func (s *AuthService) GetPlayer(ctx context.Context, id string) (*PlayerResponse, error) {
	player, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// This should be very rare — it means the JWT references a player that
			// was deleted after the token was issued.
			return nil, fmt.Errorf("AuthService.GetPlayer: player %s not found (deleted account?)", id)
		}
		return nil, fmt.Errorf("AuthService.GetPlayer: %w", err)
	}
	return player.ToResponse(), nil
}

// generateToken creates and signs a JWT for the given player.
// The token expires in 24 hours — after that the player must log in again.
//
// Parameters:
//   - playerID: embedded in the token so future requests know who the player is
//   - role: embedded so the middleware can enforce permissions without a DB lookup
func (s *AuthService) generateToken(playerID, role string) (string, error) {
	now := time.Now()

	claims := Claims{
		PlayerID: playerID,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			// ExpiresAt is when the token stops being valid.
			// jwt.NewNumericDate converts time.Time to the JWT numeric date format.
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "ark8de-l33tbo4rd",
		},
	}

	// jwt.NewWithClaims creates an unsigned token using the HS256 algorithm.
	// HS256 = HMAC-SHA256 — a symmetric algorithm that uses the same secret
	// key to both sign and verify tokens. Fast and suitable for our single-service setup.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// SignedString signs the token with our secret and produces the final
	// JWT string in the format: base64(header).base64(payload).base64(signature)
	signed, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return "", fmt.Errorf("generateToken: failed to sign JWT: %w", err)
	}

	return signed, nil
}

// --- Validation helpers ---
// These are unexported (lowercase names) — they are implementation details
// of this package and callers don't need to use them directly.

// validateUsername checks that the username is 3–50 characters, letters/digits/underscores only.
func validateUsername(username string) error {
	u := strings.TrimSpace(username)
	if len(u) < 3 || len(u) > 50 {
		return ErrInvalidUsername
	}
	// range over a string iterates over Unicode code points (runes), not bytes.
	// This handles non-ASCII letters correctly.
	for _, ch := range u {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '_' {
			return ErrInvalidUsername
		}
	}
	return nil
}

// validateEmail performs a minimal sanity check — a valid email must have an "@"
// and the domain part must contain a ".". Full RFC 5321 parsing is overkill here.
func validateEmail(email string) error {
	e := strings.TrimSpace(email)
	atIdx := strings.Index(e, "@")
	if atIdx < 1 { // atIdx < 1 means no "@", or "@" is the first character
		return ErrInvalidEmail
	}
	domain := e[atIdx+1:] // everything after "@"
	if !strings.Contains(domain, ".") || len(domain) < 3 {
		return ErrInvalidEmail
	}
	return nil
}

// validatePassword checks for minimum length and character variety.
// Requirements: ≥8 chars, at least one uppercase, one lowercase, one digit.
func validatePassword(password string) error {
	if len(password) < 8 {
		return ErrWeakPassword
	}
	var hasUpper, hasLower, hasDigit bool
	for _, ch := range password {
		// unicode.IsUpper / IsLower / IsDigit work correctly for all Unicode characters.
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit {
		return ErrWeakPassword
	}
	return nil
}

// isDuplicateConstraint checks whether a database error is a unique constraint
// violation (PostgreSQL error code "23505") on the named constraint.
//
// errors.As walks the error chain looking for a *pgconn.PgError — the concrete
// PostgreSQL error type that carries the error code and constraint name.
// If found, it fills pgErr and we can inspect the specific cause.
func isDuplicateConstraint(err error, constraintName string) bool {
	var pgErr *pgconn.PgError
	// errors.As is like errors.Is but for type matching — it unwraps the error
	// chain until it finds a value of type *pgconn.PgError, then assigns it to pgErr.
	if errors.As(err, &pgErr) {
		// "23505" is PostgreSQL's SQLSTATE code for unique_violation.
		return pgErr.Code == "23505" && pgErr.ConstraintName == constraintName
	}
	return false
}
