// Package session — Business Logic / Service layer for sessions.
//
// This file is ONLY responsible for:
//   - Generating cryptographically secure session IDs
//   - Creating sessions with computed expiry times
//   - Validating session state (exists? expired?)
//   - Destroying sessions (logout)
//
// It must NOT contain SQL queries or HTTP code.
package session

import (
	// --- Standard library ---
	"context"       // for passing context to repository calls
	"crypto/rand"   // for generating cryptographically secure random bytes
	"encoding/hex"  // for encoding random bytes as a hex string
	"errors"        // for checking sentinel errors with errors.Is
	"fmt"           // for wrapping errors with context
	"time"          // for computing session expiry
)

// Repository defines the data-access methods that SessionService needs.
//
// By depending on this interface instead of the concrete *SessionRepository,
// the service can be unit-tested with a mock repository that returns
// hardcoded data — no database needed.
//
// Go idiom: "accept interfaces, return structs."
// The interface is defined here (in the consumer package) because the consumer
// knows what it needs. The concrete SessionRepository in repository.go
// satisfies this interface automatically via Go's structural typing.
type Repository interface {
	Create(ctx context.Context, s *Session) error
	GetByID(ctx context.Context, id string) (*Session, error)
	Delete(ctx context.Context, id string) error
	DeleteByPlayerID(ctx context.Context, playerID string) error
	DeleteExpired(ctx context.Context) error
}

// SessionService contains the business logic for session management.
// It sits between the HTTP middleware/handlers and the repository.
type SessionService struct {
	repo Repository
}

// NewSessionService creates a SessionService with the given repository.
func NewSessionService(repo Repository) *SessionService {
	return &SessionService{repo: repo}
}

// CreateSession generates a new session for a player who just logged in.
//
// It creates a cryptographically random session ID, sets the expiry to
// SessionDuration from now, and stores the session in the database.
//
// Parameters:
//   - playerID: the UUID of the player who is logging in
//   - role: the player's current role (cached in the session for fast middleware checks)
//   - username: the player's username (cached for navbar display)
//
// Returns the created Session (including the ID to set as a cookie).
func (s *SessionService) CreateSession(ctx context.Context, playerID, role, username string) (*Session, error) {
	// Generate a cryptographically random session ID.
	// crypto/rand reads from the OS's secure random source (/dev/urandom on Linux,
	// CryptGenRandom on Windows). This is NOT the same as math/rand, which is
	// predictable and must NEVER be used for security-sensitive values.
	idBytes := make([]byte, 32) // 32 bytes = 256 bits of entropy
	if _, err := rand.Read(idBytes); err != nil {
		return nil, fmt.Errorf("SessionService.CreateSession: failed to generate session ID: %w", err)
	}

	// hex.EncodeToString converts bytes to a hex string: 32 bytes → 64 hex chars.
	// Hex encoding is safe for cookies and database TEXT columns.
	sessionID := hex.EncodeToString(idBytes)

	now := time.Now()
	sess := &Session{
		ID:        sessionID,
		PlayerID:  playerID,
		Role:      role,
		Username:  username,
		CreatedAt: now,
		ExpiresAt: now.Add(SessionDuration),
	}

	if err := s.repo.Create(ctx, sess); err != nil {
		return nil, fmt.Errorf("SessionService.CreateSession: %w", err)
	}

	return sess, nil
}

// GetSession retrieves and validates a session by its ID.
// Returns the session if it exists and has not expired.
// Returns ErrSessionNotFound if the ID doesn't exist, or ErrSessionExpired if past expiry.
func (s *SessionService) GetSession(ctx context.Context, id string) (*Session, error) {
	sess, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("SessionService.GetSession: %w", err)
	}

	// Check expiry. Even though the database might have expired rows that haven't
	// been cleaned up yet, we always check here to ensure we never return a stale session.
	if time.Now().After(sess.ExpiresAt) {
		// Clean up the expired session in the background — don't block the request.
		// Ignoring the error here is acceptable: the session is already rejected,
		// and a periodic cleanup job will catch any stragglers.
		_ = s.repo.Delete(ctx, id)
		return nil, ErrSessionExpired
	}

	return sess, nil
}

// DestroySession deletes a single session by its ID.
// Called when the player clicks "Logout" — only their current session is destroyed.
func (s *SessionService) DestroySession(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("SessionService.DestroySession: %w", err)
	}
	return nil
}

// DestroyAllSessions deletes all sessions for a player.
// Used for "logout everywhere" or after a password change.
func (s *SessionService) DestroyAllSessions(ctx context.Context, playerID string) error {
	if err := s.repo.DeleteByPlayerID(ctx, playerID); err != nil {
		return fmt.Errorf("SessionService.DestroyAllSessions: %w", err)
	}
	return nil
}

// CleanExpired removes all expired sessions from the database.
// Should be called periodically (e.g. once per hour) to prevent table bloat.
func (s *SessionService) CleanExpired(ctx context.Context) error {
	if err := s.repo.DeleteExpired(ctx); err != nil {
		return fmt.Errorf("SessionService.CleanExpired: %w", err)
	}
	return nil
}
