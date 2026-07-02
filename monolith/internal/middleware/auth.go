// Package middleware provides reusable HTTP middleware for the Ark8de monolith.
//
// WHAT IS MIDDLEWARE?
// Middleware is a function that wraps an HTTP handler, adding behaviour that
// runs before or after the handler. Think of it as a pipeline:
//
//	Request → [Middleware A] → [Middleware B] → [Handler] → Response
//
// In chi, middleware is applied with r.Use() (for all routes in a group)
// or r.With() (for a single route). This file provides:
//
//   - Authenticate: verifies the JWT in the Authorization header and stores
//     the player's ID and role in the request context
//   - RequireRole: checks that the authenticated player has one of the required roles
//
// Both middlewares communicate with handlers via the request Context.
// Context is Go's mechanism for passing request-scoped values (like "who is this player?")
// through the call stack without threading them through every function signature.
package middleware

import (
	// --- Standard library ---
	"context"       // for storing values in the request context
	"encoding/json" // for writing JSON error responses
	"fmt"           // for formatting error messages in the JWT key function
	"log/slog"      // structured logging — used for debugging invalid tokens
	"net/http"      // for http.Handler, http.HandlerFunc, http.ResponseWriter, http.Request
	"strings"       // for parsing "Bearer <token>" from the Authorization header

	// --- Third-party ---
	// golang-jwt/jwt is the library for parsing and validating JWT tokens.
	"github.com/golang-jwt/jwt/v5"

	// --- Internal packages ---
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/session" // server-side session lookup for cookie-based auth
)

// contextKey is a custom unexported type used as keys in the request context.
//
// WHY A CUSTOM TYPE?
// context.WithValue accepts any comparable type as a key. If we used plain
// strings like "player_id", another package could accidentally use the same
// string key and overwrite our value. A custom unexported type (lowercase `c`)
// means only this package can create values of type contextKey — our keys
// are guaranteed not to collide with any other package's context keys.
type contextKey string

const (
	// contextKeyPlayerID is the key for storing the authenticated player's UUID.
	contextKeyPlayerID contextKey = "player_id"

	// contextKeyRole is the key for storing the authenticated player's role.
	contextKeyRole contextKey = "role"

	// contextKeyUsername is the key for storing the authenticated player's username.
	// Used by the frontend to display the player's name in the navbar.
	contextKeyUsername contextKey = "username"
)

// jwtClaims mirrors the Claims struct in auth/service.go.
// We redefine it here to avoid the auth package importing middleware and
// the middleware package importing auth — that would be a circular import,
// which Go does not allow.
//
// Both structs must stay in sync. If you change one, change the other.
type jwtClaims struct {
	PlayerID string `json:"player_id"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// Authenticate returns a chi-compatible middleware function that:
//  1. Reads the JWT from the "Authorization: Bearer <token>" header
//  2. Validates the token's signature and expiry
//  3. Extracts the player's ID and role from the token claims
//  4. Stores them in the request context for downstream handlers to use
//  5. Calls the next handler if valid, or writes a 401 and stops if not
//
// Parameters:
//   - jwtSecret: the signing key — must match the key used in auth/service.go
//
// Usage in chi:
//
//	// Apply to a single route:
//	r.With(middleware.Authenticate(secret)).Get("/auth/me", handler.Me)
//
//	// Apply to a whole group:
//	r.Group(func(r chi.Router) {
//	    r.Use(middleware.Authenticate(secret))
//	    r.Get("/auth/me", handler.Me)
//	})
func Authenticate(jwtSecret string) func(http.Handler) http.Handler {
	// Authenticate returns a function because chi middleware follows the shape:
	//   func(http.Handler) http.Handler
	// The returned function receives the "next" handler in the chain and wraps it.
	// jwtSecret is captured in the closure — it stays available in the inner
	// function even after Authenticate() has returned. This is a Go closure.
	return func(next http.Handler) http.Handler {
		// http.HandlerFunc is a type adapter — it converts a plain function with
		// the signature func(ResponseWriter, *Request) into an http.Handler interface.
		// This lets us return an anonymous function as a handler without defining a struct.
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Step 1: read the Authorization header.
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeError(w, http.StatusUnauthorized, "missing Authorization header — include: Authorization: Bearer <your_token>")
				return
			}

			// Step 2: extract the token string.
			// strings.CutPrefix returns (after, true) if the string starts with the prefix,
			// or ("", false) if not. This replaces a clunky strings.HasPrefix + slice.
			tokenString, ok := strings.CutPrefix(authHeader, "Bearer ")
			if !ok || tokenString == "" {
				writeError(w, http.StatusUnauthorized, "Authorization header must be in format: Bearer <token>")
				return
			}

			// Step 3: parse and validate the JWT.
			// jwt.ParseWithClaims does three things:
			//   a) Splits the token into header.payload.signature
			//   b) Calls our keyFunc to get the signing key
			//   c) Verifies the signature and the standard claims (e.g. ExpiresAt)
			token, err := jwt.ParseWithClaims(tokenString, &jwtClaims{}, func(t *jwt.Token) (any, error) {
				// The keyFunc is called with the partially-parsed token so we can
				// verify the signing algorithm before returning the key.
				// This prevents algorithm confusion attacks — an attacker could
				// send a token with alg:"none" to bypass signature verification.
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing algorithm: %v — expected HS256", t.Header["alg"])
				}
				// Return the signing key as a []byte — jwt uses it to verify the signature.
				return []byte(jwtSecret), nil
			})

			if err != nil {
				// Log at Debug level — invalid tokens are common (expired, tampered)
				// and don't indicate a server problem. Info/Error level would flood logs.
				slog.Debug("Authenticate: token validation failed", "error", err)
				writeError(w, http.StatusUnauthorized, "invalid or expired token — please log in again")
				return
			}

			// Step 4: extract the claims.
			// token.Claims is of type jwt.Claims (an interface). We need our concrete
			// *jwtClaims type to access PlayerID and Role.
			// Type assertion syntax: value.(Type) — returns (result, ok).
			// The two-value form (with ok) is safe — it never panics, even if the
			// assertion fails. A one-value assertion without ok would panic on failure.
			claims, ok := token.Claims.(*jwtClaims)
			if !ok || !token.Valid {
				writeError(w, http.StatusUnauthorized, "invalid token claims")
				return
			}

			// Step 5: store the player's identity in the context.
			// context.WithValue creates a new context with the added key-value pair.
			// We chain two calls to add both player ID and role.
			// r.WithContext returns a shallow copy of the request with the new context —
			// we pass this enriched request to the next handler.
			ctx := context.WithValue(r.Context(), contextKeyPlayerID, claims.PlayerID)
			ctx = context.WithValue(ctx, contextKeyRole, claims.Role)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns middleware that enforces the player's role.
// It must be chained AFTER Authenticate, since it reads the role that
// Authenticate stored in the context.
//
// Parameters:
//   - roles: the permission tiers allowed to access this route.
//     Pass multiple roles to allow any of them: RequireRole("moderator", "admin")
//
// Usage:
//
//	r.With(middleware.Authenticate(secret), middleware.RequireRole("admin")).Delete(...)
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	// Build a set of allowed roles for O(1) lookup.
	// A map[string]struct{} is the idiomatic Go "set" — we use the empty struct
	// as a value because it consumes zero bytes; we only care about the keys.
	// (A map[string]bool works too and is slightly more readable — used here for clarity.)
	allowed := make(map[string]bool, len(roles))
	for _, role := range roles {
		allowed[role] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := RoleFromContext(r.Context())
			if role == "" {
				// No role in context means Authenticate wasn't applied first.
				// This is a programming error, not a client error, but we
				// return 401 to the client (and rely on developer testing to catch it).
				writeError(w, http.StatusUnauthorized, "not authenticated")
				return
			}
			if !allowed[role] {
				// 403 Forbidden: authenticated but not authorised.
				// 401 means "who are you?"; 403 means "I know who you are, but you can't do this."
				writeError(w, http.StatusForbidden, "you do not have permission to perform this action")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// PlayerIDFromContext retrieves the authenticated player's UUID from the context.
// Returns an empty string if the Authenticate middleware wasn't applied.
//
// This is exported (uppercase P) so handler packages can call it to find out
// which player made the current request.
func PlayerIDFromContext(ctx context.Context) string {
	// Type assertion with the two-value form: safe, never panics.
	// If the key is absent or not a string, ok is false and id is "".
	id, _ := ctx.Value(contextKeyPlayerID).(string)
	return id
}

// RoleFromContext retrieves the authenticated player's role from the context.
// Returns an empty string if not authenticated.
func RoleFromContext(ctx context.Context) string {
	role, _ := ctx.Value(contextKeyRole).(string)
	return role
}

// UsernameFromContext retrieves the authenticated player's username from the context.
// Returns an empty string if not authenticated. Used by the frontend to show
// the player's name in the navbar without an extra DB lookup.
func UsernameFromContext(ctx context.Context) string {
	username, _ := ctx.Value(contextKeyUsername).(string)
	return username
}

// AuthenticateWithSessions returns middleware that supports TWO authentication methods:
//
//  1. Cookie-based sessions (for browser requests) — checks the "session_id" cookie
//  2. Bearer JWT tokens (for API requests) — checks the Authorization header
//
// The middleware tries the cookie FIRST (because browsers always send cookies).
// If no cookie is found, it falls back to the Bearer token.
//
// This dual approach keeps backward compatibility: all existing JSON API clients
// (curl, tests, mobile apps) continue to use Bearer tokens, while the new
// browser-based login uses cookies.
//
// Parameters:
//   - jwtSecret: the signing key for JWT validation (fallback path)
//   - sessionSvc: the session service for cookie-based lookups
func AuthenticateWithSessions(jwtSecret string, sessionSvc *session.SessionService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// --- Path 1: try cookie-based session ---
			cookie, err := r.Cookie(session.CookieName)
			if err == nil && cookie.Value != "" {
				sess, err := sessionSvc.GetSession(r.Context(), cookie.Value)
				if err == nil {
					// Valid session found — inject player identity into the context.
					ctx := context.WithValue(r.Context(), contextKeyPlayerID, sess.PlayerID)
					ctx = context.WithValue(ctx, contextKeyRole, sess.Role)
					ctx = context.WithValue(ctx, contextKeyUsername, sess.Username)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				// Session invalid or expired — clear the stale cookie so the browser
				// stops sending it on every request.
				slog.Debug("AuthenticateWithSessions: session invalid", "error", err)
				http.SetCookie(w, &http.Cookie{
					Name:     session.CookieName,
					Value:    "",
					Path:     "/",
					MaxAge:   -1, // -1 tells the browser to delete the cookie immediately
					HttpOnly: true,
				})
			}

			// --- Path 2: fall back to Bearer JWT ---
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeError(w, http.StatusUnauthorized, "not authenticated — please log in")
				return
			}

			tokenString, ok := strings.CutPrefix(authHeader, "Bearer ")
			if !ok || tokenString == "" {
				writeError(w, http.StatusUnauthorized, "Authorization header must be in format: Bearer <token>")
				return
			}

			token, err := jwt.ParseWithClaims(tokenString, &jwtClaims{}, func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing algorithm: %v — expected HS256", t.Header["alg"])
				}
				return []byte(jwtSecret), nil
			})
			if err != nil {
				slog.Debug("AuthenticateWithSessions: JWT validation failed", "error", err)
				writeError(w, http.StatusUnauthorized, "invalid or expired token — please log in again")
				return
			}

			claims, ok := token.Claims.(*jwtClaims)
			if !ok || !token.Valid {
				writeError(w, http.StatusUnauthorized, "invalid token claims")
				return
			}

			ctx := context.WithValue(r.Context(), contextKeyPlayerID, claims.PlayerID)
			ctx = context.WithValue(ctx, contextKeyRole, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuthenticateWithSessions is like AuthenticateWithSessions but does NOT
// return 401 if no session or token is found. It simply passes the request through.
//
// WHY THIS EXISTS:
//   Public pages (leaderboard, team detail) don't require authentication, but we
//   still want the navbar to show "Logged in as <username>" when the player HAS
//   a valid session cookie. This middleware reads the session if present and
//   injects identity into the context, but always calls next.ServeHTTP — even
//   if no authentication is found.
//
// Usage: apply to all frontend HTML routes so the base template can conditionally
// show login/logout links.
func OptionalAuthenticateWithSessions(jwtSecret string, sessionSvc *session.SessionService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try cookie-based session first.
			cookie, err := r.Cookie(session.CookieName)
			if err == nil && cookie.Value != "" {
				sess, err := sessionSvc.GetSession(r.Context(), cookie.Value)
				if err == nil {
					ctx := context.WithValue(r.Context(), contextKeyPlayerID, sess.PlayerID)
					ctx = context.WithValue(ctx, contextKeyRole, sess.Role)
					ctx = context.WithValue(ctx, contextKeyUsername, sess.Username)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				// Session invalid — clear stale cookie but continue (don't block).
				http.SetCookie(w, &http.Cookie{
					Name:     session.CookieName,
					Value:    "",
					Path:     "/",
					MaxAge:   -1,
					HttpOnly: true,
				})
			}

			// Try Bearer token (for cases where an API client hits an HTML page).
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				tokenString, ok := strings.CutPrefix(authHeader, "Bearer ")
				if ok && tokenString != "" {
					token, err := jwt.ParseWithClaims(tokenString, &jwtClaims{}, func(t *jwt.Token) (any, error) {
						if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
							return nil, fmt.Errorf("unexpected signing algorithm: %v", t.Header["alg"])
						}
						return []byte(jwtSecret), nil
					})
					if err == nil {
						if claims, ok := token.Claims.(*jwtClaims); ok && token.Valid {
							ctx := context.WithValue(r.Context(), contextKeyPlayerID, claims.PlayerID)
							ctx = context.WithValue(ctx, contextKeyRole, claims.Role)
							next.ServeHTTP(w, r.WithContext(ctx))
							return
						}
					}
				}
			}

			// No valid authentication found — continue anyway (this is optional auth).
			next.ServeHTTP(w, r)
		})
	}
}

// writeError writes a JSON error response from within the middleware.
// Defined locally because the middleware package does not import any handler
// packages (that would create an import cycle).
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Intentionally ignore the encode error here — if we can't write the response,
	// there's nothing meaningful we can do about it.
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
