// Package middleware — Flash message system.
//
// WHAT ARE FLASH MESSAGES?
//   A flash message is a one-time notification shown to the user after an action.
//   For example: "Account created!" after registering, or "Invalid password" after
//   a failed login attempt.
//
//   "Flash" means the message appears once and disappears on the next page load.
//   This is the "read-once" pattern — the message is consumed when it's displayed.
//
// HOW IT WORKS:
//   1. A handler calls SetFlash(w, "success", "Account created!") before redirecting.
//   2. SetFlash writes a short-lived cookie containing the message.
//   3. The browser follows the redirect and sends the cookie back on the next request.
//   4. FlashFromRequest reads the cookie, returns the message, and tells the browser
//      to delete the cookie (MaxAge=-1).
//   5. The template displays the message. On the next page load, the cookie is gone.
//
// WHY A COOKIE (not a session field)?
//   Keeping flash messages in cookies is stateless — no extra DB write for a
//   temporary notification. The message is small (< 200 bytes) and expires in
//   10 seconds, so even if the user never loads another page, it auto-cleans.
package middleware

import (
	// --- Standard library ---
	"encoding/base64" // for encoding the flash JSON safely in a cookie value
	"encoding/json"   // for serializing the flash message as JSON
	"net/http"        // for http.Cookie, http.ResponseWriter, http.Request
)

// flashCookieName is the name of the cookie that carries the flash message.
const flashCookieName = "flash"

// FlashMessage represents a one-time notification to display to the user.
type FlashMessage struct {
	// Type is the category of the message: "success", "error", "info".
	// Templates use this to apply the correct CSS class (green, red, blue).
	Type string `json:"type"`

	// Message is the human-readable text shown to the user.
	Message string `json:"message"`
}

// SetFlash writes a flash message cookie that will be read on the next request.
//
// The message is JSON-encoded and then base64-encoded because cookie values
// cannot contain certain characters (spaces, commas, semicolons). Base64
// produces a safe, ASCII-only string.
//
// MaxAge=10 means the cookie expires in 10 seconds. This is a safety net:
// if the user doesn't load another page (which would consume the flash),
// the cookie self-destructs instead of persisting forever.
//
// Parameters:
//   - w: the ResponseWriter to set the cookie on
//   - flashType: "success", "error", or "info"
//   - message: the text to display
func SetFlash(w http.ResponseWriter, flashType, message string) {
	flash := FlashMessage{Type: flashType, Message: message}

	// json.Marshal converts the struct to a JSON byte slice: {"type":"success","message":"..."}
	jsonBytes, err := json.Marshal(flash)
	if err != nil {
		// If we can't marshal a simple struct, something is very wrong.
		// Skip the flash rather than crashing — the user just won't see the notification.
		return
	}

	// base64.URLEncoding produces URL-safe base64 (uses - and _ instead of + and /).
	// This is important because cookie values must be URL-safe.
	encoded := base64.URLEncoding.EncodeToString(jsonBytes)

	http.SetCookie(w, &http.Cookie{
		Name:     flashCookieName,
		Value:    encoded,
		Path:     "/",
		MaxAge:   10, // 10 seconds — just long enough for one redirect cycle
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// FlashFromRequest reads the flash cookie from the request and returns the
// decoded message. If no flash cookie exists, returns nil.
//
// IMPORTANT: This function does NOT delete the cookie. The caller must call
// ClearFlash(w) after reading, or the same flash will appear on the next request too.
// In practice, the template rendering code calls FlashFromRequest to get the message
// and ClearFlash to delete the cookie, ensuring the "read-once" behaviour.
func FlashFromRequest(r *http.Request) *FlashMessage {
	cookie, err := r.Cookie(flashCookieName)
	if err != nil || cookie.Value == "" {
		return nil
	}

	// Decode base64 → JSON bytes → FlashMessage struct.
	jsonBytes, err := base64.URLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return nil // corrupted cookie — ignore it
	}

	var flash FlashMessage
	if err := json.Unmarshal(jsonBytes, &flash); err != nil {
		return nil // corrupted JSON — ignore it
	}

	return &flash
}

// ClearFlash deletes the flash cookie by setting MaxAge=-1.
// Call this after reading the flash to implement the "read-once" pattern.
func ClearFlash(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1, // -1 tells the browser to delete the cookie immediately
		HttpOnly: true,
	})
}
