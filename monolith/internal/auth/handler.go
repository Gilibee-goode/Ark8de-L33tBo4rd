// Package auth — HTTP Handler layer
//
// This file is ONLY responsible for:
//   - Parsing data from the HTTP request (JSON body, headers)
//   - Calling AuthService to do the actual work
//   - Writing the HTTP response (status code + JSON body)
//
// It must NOT contain:
//   - Business logic (validation rules, game logic) → that belongs in service.go
//   - SQL queries → that belongs in repository.go
//
// If you find yourself writing an if-statement about game rules here, move it to service.go.
package auth

import (
	// --- Standard library ---
	"encoding/json" // for reading JSON request bodies (json.NewDecoder) and writing JSON responses
	"errors"        // for comparing errors with errors.Is() to return the right HTTP status
	"log/slog"      // structured logging — used for unexpected errors that we don't expose to the client
	"net/http"      // the Go standard HTTP library — provides ResponseWriter, Request, and status codes

	// --- Internal packages ---
	// respond provides the shared JSON response helpers used across all handlers.
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/respond"
	// middleware provides PlayerIDFromContext — reads the player's ID from the request
	// context, where the JWT middleware stored it after validating the token.
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/middleware"
)

// AuthHandler handles HTTP requests for the /auth/* endpoints.
// It holds a reference to the service layer, which contains the actual business logic.
// The handler's job is to translate between HTTP and Go function calls.
type AuthHandler struct {
	// service is the business logic layer. The handler calls it and translates
	// the result into an HTTP response.
	service *AuthService
}

// NewAuthHandler creates and returns an AuthHandler.
// Called once at startup in main.go with the already-constructed AuthService.
func NewAuthHandler(service *AuthService) *AuthHandler {
	return &AuthHandler{service: service}
}

// Register handles POST /auth/register.
// It reads the new account details from the JSON body, calls the service to
// create the account, and responds with the player's profile and a JWT.
//
// Response codes:
//   - 201 Created     — account created successfully; body: AuthResponse{token, player}
//   - 400 Bad Request — malformed JSON, weak password, invalid username/email
//   - 409 Conflict    — email or username already taken
//   - 500 Internal    — unexpected server error (details logged, not exposed)
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	// json.NewDecoder reads the request body as a stream and decodes it into
	// our RegisterRequest struct. This is more memory-efficient than reading
	// the entire body into a []byte first, especially for large payloads.
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body — expected JSON: {\"username\": \"...\", \"email\": \"...\", \"password\": \"...\"}")
		return
		// `return` stops handler execution here — the deferred function and
		// everything below this point will not run.
	}

	player, token, err := h.service.Register(r.Context(), req)
	if err != nil {
		// Map each known service error to the appropriate HTTP status code.
		// This switch uses errors.Is() which safely traverses wrapped error chains.
		switch {
		case errors.Is(err, ErrInvalidUsername),
			errors.Is(err, ErrInvalidEmail),
			errors.Is(err, ErrWeakPassword):
			// 400 Bad Request — the input didn't meet our requirements.
			respond.Error(w, http.StatusBadRequest, err.Error())

		case errors.Is(err, ErrEmailTaken),
			errors.Is(err, ErrUsernameTaken):
			// 409 Conflict — a resource with that identifier already exists.
			respond.Error(w, http.StatusConflict, err.Error())

		default:
			// Unexpected error — log it server-side with full details for debugging,
			// but return a generic message to the client. Never expose internal
			// error details (stack traces, DB errors) to the outside world.
			slog.Error("AuthHandler.Register: unexpected error", "error", err)
			respond.Error(w, http.StatusInternalServerError, "an unexpected error occurred — please try again")
		}
		return
	}

	// 201 Created — the standard success code for resource creation (not 200 OK).
	respond.JSON(w, http.StatusCreated, AuthResponse{Token: token, Player: player})
}

// Login handles POST /auth/login.
// It reads the credentials, verifies them via the service, and returns a JWT.
//
// Response codes:
//   - 200 OK           — credentials valid; body: AuthResponse{token, player}
//   - 400 Bad Request  — malformed JSON body
//   - 401 Unauthorized — email not found, or password doesn't match
//   - 500 Internal     — unexpected server error
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body — expected JSON: {\"email\": \"...\", \"password\": \"...\"}")
		return
	}

	player, token, err := h.service.Login(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			// 401 Unauthorized — the standard code for failed authentication.
			// We use the same message for "email not found" and "wrong password"
			// (see service.go) to avoid leaking which one failed.
			respond.Error(w, http.StatusUnauthorized, err.Error())
			return
		}
		slog.Error("AuthHandler.Login: unexpected error", "error", err)
		respond.Error(w, http.StatusInternalServerError, "an unexpected error occurred — please try again")
		return
	}

	respond.JSON(w, http.StatusOK, AuthResponse{Token: token, Player: player})
}

// Me handles GET /auth/me.
// Returns the full profile of the currently authenticated player.
//
// This endpoint requires a valid JWT. If the token is missing or expired,
// the Authenticate middleware (in middleware/auth.go) will return 401
// before this handler even runs.
//
// Response codes:
//   - 200 OK           — body: PlayerResponse
//   - 401 Unauthorized — no token / invalid token (handled by middleware, not here)
//   - 500 Internal     — unexpected server error
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	// The JWT middleware extracted the player's ID from the token and stored
	// it in the request context. We read it back here.
	// If the route is properly protected by middleware, this will always be non-empty.
	playerID := middleware.PlayerIDFromContext(r.Context())
	if playerID == "" {
		// Defensive guard — should not reach here if middleware is wired correctly.
		respond.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	player, err := h.service.GetPlayer(r.Context(), playerID)
	if err != nil {
		slog.Error("AuthHandler.Me: failed to fetch player", "player_id", playerID, "error", err)
		respond.Error(w, http.StatusInternalServerError, "failed to load player profile")
		return
	}

	respond.JSON(w, http.StatusOK, player)
}
