// Package tests — Auth endpoint integration tests
//
// This file tests the authentication endpoints:
//   POST /auth/register — create a new player account
//   POST /auth/login    — authenticate and receive a JWT
//   GET  /auth/me       — fetch the current player's profile (requires JWT)
//
// Each test function follows this pattern:
//   1. Set up any required state (register a player, get a token)
//   2. Make an HTTP request to the test server
//   3. Assert the status code and response body
//
// WHAT IS A SUBTEST?
//   t.Run("name", func(t *testing.T) { ... }) creates a subtest.
//   Subtests let you group related checks under one parent test.
//   When you run `go test -v`, you'll see output like:
//     TestAuthRegister/success
//     TestAuthRegister/duplicate_email
//   This makes it easy to see which specific scenario failed.
package tests

import (
	// --- Standard library ---
	"encoding/json" // json.Marshal builds JSON request bodies from Go maps
	"net/http"      // HTTP status constants like http.StatusCreated, http.StatusConflict
	"testing"       // Go's test framework — provides *testing.T for writing test assertions
)

// ---------------------------------------------------------------------------
// POST /auth/register
// ---------------------------------------------------------------------------

// TestAuthRegister tests player registration with valid and invalid inputs.
//
// In Go, any function named Test<Something>(t *testing.T) is automatically
// recognised as a test function. The testing framework calls it when you
// run `go test`. The `t` parameter provides methods like t.Run (subtests),
// t.Fatalf (fail and stop), t.Errorf (fail but continue), etc.
func TestAuthRegister(t *testing.T) {

	// ---- Subtest: successful registration ----
	// A brand new player should get 201 Created with a JWT token and player info.
	t.Run("success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"username": "test_register",
			"email":    "register@test.dev",
			"password": "TestPass1!",
		})

		resp := doRequest(t, "POST", "/auth/register", body, "")
		assertStatus(t, resp, http.StatusCreated)

		result := parseJSON(t, resp)

		// The response should contain a "token" field (the JWT) and a "player" object.
		if _, ok := result["token"]; !ok {
			t.Fatal("expected 'token' field in register response")
		}

		// Check that the player object has the expected username.
		// We navigate into the nested JSON: {"player": {"username": "test_register"}}.
		player := result["player"].(map[string]any)
		if player["username"] != "test_register" {
			t.Fatalf("expected username 'test_register', got %v", player["username"])
		}

		// New players should have the default role "player" (not moderator or admin).
		if player["role"] != "player" {
			t.Fatalf("expected role 'player', got %v", player["role"])
		}
	})

	// ---- Subtest: duplicate email ----
	// Registering with an email that already exists should return 409 Conflict.
	t.Run("duplicate_email", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"username": "another_user",
			"email":    "register@test.dev", // same email as above
			"password": "TestPass1!",
		})

		resp := doRequest(t, "POST", "/auth/register", body, "")
		assertStatus(t, resp, http.StatusConflict)
	})

	// ---- Subtest: duplicate username ----
	// Registering with a username that already exists should also return 409 Conflict.
	t.Run("duplicate_username", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"username": "test_register", // same username as the first subtest
			"email":    "different@test.dev",
			"password": "TestPass1!",
		})

		resp := doRequest(t, "POST", "/auth/register", body, "")
		assertStatus(t, resp, http.StatusConflict)
	})

	// ---- Subtest: weak password ----
	// A password that doesn't meet the strength requirements should return 400 Bad Request.
	// Requirements: 8+ chars, one uppercase, one lowercase, one digit.
	t.Run("weak_password", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"username": "weak_pw_user",
			"email":    "weakpw@test.dev",
			"password": "short", // too short, no uppercase, no digit
		})

		resp := doRequest(t, "POST", "/auth/register", body, "")
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// ---- Subtest: missing fields ----
	// An empty body or missing required fields should return 400 Bad Request.
	t.Run("missing_fields", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"username": "", // empty
			"email":    "",
			"password": "",
		})

		resp := doRequest(t, "POST", "/auth/register", body, "")
		assertStatus(t, resp, http.StatusBadRequest)
	})
}

// ---------------------------------------------------------------------------
// POST /auth/login
// ---------------------------------------------------------------------------

// TestAuthLogin tests the login endpoint with correct and incorrect credentials.
func TestAuthLogin(t *testing.T) {

	// ---- Setup: create a player to log in as ----
	// We register a fresh player so we know the exact credentials.
	registerPlayer(t, "login_user", "login@test.dev", "LoginPass1!")

	// ---- Subtest: successful login ----
	t.Run("success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"email":    "login@test.dev",
			"password": "LoginPass1!",
		})

		resp := doRequest(t, "POST", "/auth/login", body, "")
		assertStatus(t, resp, http.StatusOK)

		result := parseJSON(t, resp)

		// Should return a token and the player's info.
		if _, ok := result["token"]; !ok {
			t.Fatal("expected 'token' field in login response")
		}

		player := result["player"].(map[string]any)
		if player["username"] != "login_user" {
			t.Fatalf("expected username 'login_user', got %v", player["username"])
		}
	})

	// ---- Subtest: wrong password ----
	t.Run("wrong_password", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"email":    "login@test.dev",
			"password": "WrongPass1!",
		})

		resp := doRequest(t, "POST", "/auth/login", body, "")
		assertStatus(t, resp, http.StatusUnauthorized)
	})

	// ---- Subtest: email not found ----
	t.Run("email_not_found", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"email":    "nobody@test.dev",
			"password": "SomePass1!",
		})

		resp := doRequest(t, "POST", "/auth/login", body, "")
		assertStatus(t, resp, http.StatusUnauthorized)
	})
}

// ---------------------------------------------------------------------------
// GET /auth/me
// ---------------------------------------------------------------------------

// TestAuthMe tests the "who am I" endpoint that requires a valid JWT.
func TestAuthMe(t *testing.T) {

	// ---- Setup: register and get a token ----
	token, _ := registerAndGetToken(t, "me_user", "me@test.dev", "MePass1!")

	// ---- Subtest: success with valid token ----
	t.Run("success", func(t *testing.T) {
		resp := doRequest(t, "GET", "/auth/me", nil, token)
		assertStatus(t, resp, http.StatusOK)

		result := parseJSON(t, resp)
		if result["username"] != "me_user" {
			t.Fatalf("expected username 'me_user', got %v", result["username"])
		}
	})

	// ---- Subtest: missing token ----
	// Without a JWT, the auth middleware should return 401 Unauthorized.
	t.Run("no_token", func(t *testing.T) {
		resp := doRequest(t, "GET", "/auth/me", nil, "")
		assertStatus(t, resp, http.StatusUnauthorized)
	})

	// ---- Subtest: invalid token ----
	// A garbage token should also return 401.
	t.Run("invalid_token", func(t *testing.T) {
		resp := doRequest(t, "GET", "/auth/me", nil, "not.a.valid.jwt")
		assertStatus(t, resp, http.StatusUnauthorized)
	})
}
