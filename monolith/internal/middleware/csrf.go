// Package middleware — CSRF (Cross-Site Request Forgery) protection.
//
// WHAT IS CSRF?
//   CSRF is an attack where a malicious website tricks a user's browser into
//   making a request to our site. Because the browser automatically attaches
//   cookies, the request looks legitimate — but the user never intended it.
//
//   Example: a hidden form on evil.com POSTs to ark8de.dev/mod/award-points.
//   If the moderator is logged in (has a session cookie), the browser sends
//   the cookie along, and the request succeeds — the attacker just awarded
//   themselves Arkade points.
//
// HOW WE PREVENT IT — Double-Submit Cookie Pattern:
//   1. On every GET request, we set a "csrf_token" cookie with a random value.
//   2. Every HTML form includes a hidden <input> with the same token value.
//   3. On POST/PUT/DELETE, we compare the form field to the cookie value.
//   4. An attacker's page can cause the browser to SEND our cookies, but it
//      CANNOT READ them (same-origin policy). So it cannot put the correct
//      token in the form field — the request is rejected.
//
// WHY NOT JUST SameSite=Lax?
//   SameSite=Lax prevents cross-site POST requests from attaching cookies,
//   which blocks most CSRF. But it has edge cases (same-site subdomains,
//   older browsers) and is defense-in-depth — not a complete solution.
//   The double-submit cookie is a belt-and-suspenders approach.
package middleware

import (
	// --- Standard library ---
	"context"      // for storing the CSRF token in the request context
	"crypto/rand"  // for generating cryptographically secure random tokens
	"encoding/hex" // for encoding random bytes as a hex string
	"net/http"     // for http.Handler, http.Cookie, http.Request, http.ResponseWriter
)

// csrfCookieName is the name of the cookie that holds the CSRF token.
// It is NOT HttpOnly — JavaScript and templates need to read it.
const csrfCookieName = "csrf_token"

// contextKeyCSRFToken is the context key for the CSRF token.
// Handlers and templates read it to embed the token in forms.
const contextKeyCSRFToken contextKey = "csrf_token"

// CSRFTokenFromContext retrieves the CSRF token from the request context.
// Templates use this to render a hidden <input> field in every form.
func CSRFTokenFromContext(ctx context.Context) string {
	token, _ := ctx.Value(contextKeyCSRFToken).(string)
	return token
}

// CSRFProtect returns middleware that enforces the double-submit cookie pattern.
//
// For safe methods (GET, HEAD, OPTIONS):
//   - Ensures a csrf_token cookie exists (generates one if missing)
//   - Stores the token in the request context for templates to read
//
// For unsafe methods (POST, PUT, DELETE, PATCH):
//   - Reads the csrf_token from the form body (or X-CSRF-Token header)
//   - Compares it to the csrf_token cookie
//   - Rejects the request with 403 if they don't match
//
// IMPORTANT: Apply this middleware ONLY to frontend (HTML) routes.
// JSON API routes use Bearer tokens and don't need CSRF protection
// (Bearer tokens are never sent automatically by the browser).
func CSRFProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the existing CSRF cookie, or generate a new one.
		cookie, err := r.Cookie(csrfCookieName)
		var csrfToken string

		if err != nil || cookie.Value == "" {
			// No CSRF cookie — generate a new token.
			csrfToken = generateCSRFToken()
			http.SetCookie(w, &http.Cookie{
				Name:     csrfCookieName,
				Value:    csrfToken,
				Path:     "/",
				MaxAge:   86400 * 7, // 7 days — matches session duration
				HttpOnly: false,     // templates need to read this value
				SameSite: http.SameSiteLaxMode,
			})
		} else {
			csrfToken = cookie.Value
		}

		// Store the token in context so templates can embed it in forms.
		ctx := context.WithValue(r.Context(), contextKeyCSRFToken, csrfToken)
		r = r.WithContext(ctx)

		// For safe methods, just store the token and pass through.
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		// For unsafe methods (POST, PUT, DELETE, PATCH), validate the token.
		// The token can come from a form field or a header (for JavaScript fetch calls).
		formToken := r.FormValue("csrf_token")
		if formToken == "" {
			formToken = r.Header.Get("X-CSRF-Token")
		}

		if formToken == "" || formToken != csrfToken {
			// Token mismatch or missing — likely a CSRF attack or a stale form.
			http.Error(w, "CSRF token mismatch — please reload the page and try again", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// generateCSRFToken creates a random 32-byte hex-encoded token.
// Uses crypto/rand for security — same approach as session ID generation.
func generateCSRFToken() string {
	b := make([]byte, 32)
	// crypto/rand.Read fills the slice with cryptographically secure random bytes.
	// The error case is extremely rare (only if the OS random source fails).
	if _, err := rand.Read(b); err != nil {
		// Fallback: return a fixed string that will cause token validation to fail.
		// This is safer than returning an empty string (which could match another empty string).
		return "csrf-generation-failed"
	}
	return hex.EncodeToString(b)
}
