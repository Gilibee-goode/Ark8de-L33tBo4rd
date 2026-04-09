// Package respond provides shared helpers for writing HTTP responses.
//
// Every handler in this monolith needs to write JSON responses and errors
// in a consistent format. Rather than duplicating this logic in every package,
// we define it once here and import it everywhere.
//
// All API responses follow two shapes:
//
//	Success: the payload directly, e.g. {"id": "...", "username": "..."}
//	Error:   {"error": "human-readable message"}
package respond

import (
	// --- Standard library ---
	"encoding/json" // for marshalling Go values to JSON and writing them to the response
	"log/slog"      // structured logging — used to log encoding failures
	"net/http"      // for http.ResponseWriter and HTTP status code constants
)

// JSON writes a JSON-encoded response with the given HTTP status code.
//
// It sets Content-Type to application/json before writing — this tells clients
// how to parse the response body. The header MUST be set before calling
// WriteHeader, because once headers are sent they cannot be changed.
//
// Parameters:
//   - w: the response writer provided by the HTTP framework
//   - status: the HTTP status code, e.g. http.StatusOK (200)
//   - body: any Go value that can be JSON-encoded (struct, map, slice, etc.)
//
// The `any` type (introduced in Go 1.18) is an alias for `interface{}` —
// it means "this parameter accepts a value of any type".
func JSON(w http.ResponseWriter, status int, body any) {
	// Set the Content-Type header before WriteHeader.
	// Once WriteHeader is called, the HTTP response line and headers are
	// flushed to the client — any header changes after that are silently ignored.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	// json.NewEncoder(w) creates an encoder that writes directly into w.
	// This is more efficient than json.Marshal (which produces a []byte)
	// because it streams JSON bytes directly to the client without buffering.
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// We can't write an error response here because the headers and status
		// code were already sent. All we can do is log it.
		slog.Error("respond.JSON: failed to encode response body", "error", err)
	}
}

// Error writes a JSON error response with a human-readable message.
//
// All error responses use the shape {"error": "message"} — a consistent
// format that clients can always rely on, regardless of which endpoint failed.
//
// Parameters:
//   - w: the response writer
//   - status: HTTP status code, e.g. http.StatusBadRequest (400)
//   - message: a human-readable explanation of what went wrong
func Error(w http.ResponseWriter, status int, message string) {
	// map[string]string is a Go map with string keys and string values.
	// Here we use it as an anonymous struct-like container to produce {"error": "..."}
	// without defining a named type.
	JSON(w, status, map[string]string{"error": message})
}
