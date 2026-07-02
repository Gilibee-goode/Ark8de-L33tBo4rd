// Package session — Data model layer for browser sessions.
//
// This package manages server-side sessions for browser-based authentication.
// When a player logs in via the HTML form, a session is created in the database
// and the session ID is sent as an HttpOnly cookie. On subsequent requests,
// the middleware reads the cookie and looks up the session to identify the player.
//
// WHY SERVER-SIDE SESSIONS (not JWT-in-cookie)?
//   JWTs are stateless — once issued, they cannot be revoked until they expire.
//   Server-side sessions can be destroyed immediately (logout, password change,
//   admin force-logout). This is the fundamental difference between stateless
//   and stateful authentication:
//     - JWT (stateless): fast, no DB lookup, but cannot revoke
//     - Session (stateful): requires DB lookup, but supports instant revocation
//
//   The JSON API continues to use JWT (for programmatic clients like curl or
//   mobile apps). The browser uses sessions (for security and UX).
package session

import (
	// --- Standard library ---
	"errors" // for defining sentinel errors
	"time"   // for session expiry timestamps
)

// Session represents an active login session stored in the database.
// Each session is tied to a player and has an expiry time.
type Session struct {
	// ID is a cryptographically random 64-character hex string (32 bytes).
	// It serves as both the primary key in the database and the value of the
	// session_id cookie sent to the browser.
	ID string

	// PlayerID is the UUID of the player who owns this session.
	PlayerID string

	// Role is cached from the player at login time (e.g. "player", "moderator", "admin").
	// Caching it here avoids a JOIN on every request. If the player's role changes,
	// they need to log out and back in for the new role to take effect.
	Role string

	// Username is cached from the player at login time.
	// Displayed in the navbar without needing an extra DB lookup per page.
	Username string

	// CreatedAt is when the session was created.
	CreatedAt time.Time

	// ExpiresAt is when this session stops being valid.
	// The middleware rejects sessions past their expiry.
	ExpiresAt time.Time
}

// Sentinel errors for the session package.
// Callers use errors.Is() to check for these specific conditions.
var (
	// ErrSessionNotFound is returned when a session ID does not exist in the database
	// or has already been deleted (e.g. after logout).
	ErrSessionNotFound = errors.New("session not found")

	// ErrSessionExpired is returned when a session exists but its expires_at
	// timestamp is in the past. The session should be cleaned up.
	ErrSessionExpired = errors.New("session has expired")
)

// SessionDuration is how long a session stays valid after creation.
// 7 days is longer than the JWT's 24-hour expiry because sessions can be
// revoked at any time (via logout), so a longer lifetime is safe.
const SessionDuration = 7 * 24 * time.Hour

// CookieName is the name of the HTTP cookie that carries the session ID.
// Using a constant prevents typos when reading/writing the cookie across packages.
const CookieName = "session_id"
