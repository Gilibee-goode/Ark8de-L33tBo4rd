// Package tests — Integration test suite for the Ark8de monolith
//
// This file is the TEST SETUP. It runs before any individual test and provides:
//   - A real PostgreSQL database (either via testcontainers or an external DB)
//   - A fully wired HTTP test server using httptest.NewServer
//   - Helper functions for making requests, parsing JSON, and asserting status codes
//
// DATABASE MODES:
//
//   1. Testcontainers (default) — a fresh PostgreSQL container is launched
//      automatically via Docker. Migrations run against it. No setup needed.
//      Just run: cd monolith && go test ./tests/... -v
//
//   2. External database (override) — set the DATABASE_URL environment variable
//      to use an existing PostgreSQL instance (e.g. docker-compose dev).
//      Run: DATABASE_URL="postgres://..." go test ./tests/... -v
//
// WHY INTEGRATION TESTS?
//   These tests exercise the full stack: HTTP request → router → handler →
//   service → repository → PostgreSQL. They catch bugs that unit tests miss —
//   SQL errors, missing columns, transaction issues, and middleware misconfig.
//
// HOW IT WORKS:
//   Go's testing package looks for a special function called TestMain(m *testing.M).
//   If it exists, Go calls TestMain INSTEAD of running tests directly.
//   Inside TestMain, we set up the database, create the test server, and then call
//   m.Run() to execute the actual test functions. After all tests finish, we clean up.
package tests

import (
	// --- Standard library ---
	"bytes"          // bytes.NewReader creates an io.Reader from a byte slice — used to build JSON request bodies
	"context"        // Context for DB operations — every DB call needs a context for cancellation/timeouts
	"encoding/json"  // encoding/decoding JSON — used to parse API responses and build request bodies
	"fmt"            // string formatting — used to build URL paths with player/team IDs
	"io"             // io.ReadAll reads an HTTP response body into a byte slice
	"net/http"       // standard HTTP client and types — used to make requests to the test server
	"net/http/httptest" // httptest.NewServer creates a local HTTP server for testing — no real port needed
	"os"             // os.Exit for TestMain, os.Getenv for DATABASE_URL
	"strings"        // strings.Contains for checking HTML response content
	"testing"        // Go's built-in test framework — provides *testing.T for assertions and *testing.M for TestMain

	// --- Third-party ---
	"github.com/jackc/pgx/v5/pgxpool" // PostgreSQL connection pool — same driver the app uses

	// --- Internal packages ---
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/app" // shared router builder — builds the same router as production
)

// ---------------------------------------------------------------------------
// Package-level variables — shared across all test files in the tests package
// ---------------------------------------------------------------------------

// testServer is the httptest server that hosts our full application router.
// Every test makes HTTP requests against this server's URL.
//
// httptest.NewServer is a Go standard library tool that starts a real HTTP
// server on a random available port (e.g. http://127.0.0.1:52431).
// It is much faster than starting a Docker container and is the standard
// way to integration-test Go HTTP applications.
var testServer *httptest.Server

// testPool is the database connection pool used by the test setup to
// truncate tables between test groups. The application router gets its
// own pool internally through app.BuildRouter.
var testPool *pgxpool.Pool

// jwtSecret is the secret used to sign JWT tokens in tests.
// It must match the secret passed to app.BuildRouter so that tokens
// generated during tests are accepted by the auth middleware.
const jwtSecret = "test-secret-for-integration-tests"

// ---------------------------------------------------------------------------
// TestMain — the entry point for the entire test suite
// ---------------------------------------------------------------------------
//
// TestMain is a special function recognised by Go's testing framework.
// If a test package defines TestMain(m *testing.M), Go calls it instead
// of running test functions directly. This gives us a place to do
// one-time setup (connect to DB, start server) and teardown (close pool).
//
// The argument `m *testing.M` represents the set of tests to run.
// Calling m.Run() executes all Test* functions in this package and
// returns an exit code (0 = all passed, 1 = some failed).
// We pass that exit code to os.Exit so the CI pipeline gets the right signal.
func TestMain(m *testing.M) {
	ctx := context.Background()

	// ---- Step 1: Change to monolith root ----
	// The working directory must be monolith/ so that:
	//   - Template paths ("templates/layout/base.html") resolve correctly
	//   - Static file paths ("static/style.css") resolve correctly
	//   - Migration paths ("../db/migrations") resolve correctly for testcontainers
	if err := os.Chdir(".."); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: cannot chdir to monolith root: %v\n", err)
		os.Exit(1)
	}

	// ---- Step 2: Get a database (testcontainers or external) ----
	//
	// If DATABASE_URL is set, use that external database directly.
	// Otherwise, spin up a fresh PostgreSQL container via testcontainers.
	//
	// The testcontainers path is the default because it requires zero setup —
	// just Docker running. The DATABASE_URL override exists for:
	//   - Local dev when you already have docker-compose running
	//   - CI environments that provide their own PostgreSQL service
	var dbURL string
	var containerCleanup func()

	dbURL = os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// No external database — launch a disposable container.
		// startPostgresContainer (in container_test.go) handles:
		//   1. Starting a postgres:16-alpine container
		//   2. Waiting for it to be ready
		//   3. Running all migrations (creates tables, indexes, seeds gear_types)
		var err error
		dbURL, containerCleanup, err = startPostgresContainer(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: cannot start test database container: %v\n", err)
			fmt.Fprintf(os.Stderr, "HINT: make sure Docker is running, or set DATABASE_URL to use an external database\n")
			os.Exit(1)
		}
		fmt.Println("✓ testcontainers: PostgreSQL container started with migrations applied")
	} else {
		fmt.Printf("✓ using external database: %s\n", dbURL)
	}

	// ---- Step 3: Connect to the database ----
	// context.Background() creates a root context with no deadline.
	// We use it here because the test setup should either succeed or fail fast.
	var err error
	testPool, err = pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: cannot connect to test database: %v\n", err)
		os.Exit(1)
	}

	// Verify the connection is alive with a ping.
	if err := testPool.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: database ping failed: %v\n", err)
		os.Exit(1)
	}

	// ---- Step 4: Build the application router ----
	// app.BuildRouter wires up the exact same dependency graph as production:
	// repos → services → handlers → chi router.
	// We pass the test database pool so the app talks to the right database.
	router := app.BuildRouter(testPool, jwtSecret, "templates")

	// ---- Step 5: Start the test HTTP server ----
	// httptest.NewServer takes any http.Handler (our chi.Router implements it)
	// and starts a real HTTP server on localhost with a random port.
	// testServer.URL will be something like "http://127.0.0.1:52431".
	testServer = httptest.NewServer(router)

	// ---- Step 6: Clean the database before tests run ----
	// We truncate all data tables to ensure tests start from a clean state.
	// Reference tables (skills, gear_types) are preserved — they're seeded
	// by migrations or the seed script and tests depend on them existing.
	truncateDataTables(ctx)

	// ---- Step 7: Run all tests ----
	exitCode := m.Run()

	// ---- Step 8: Teardown ----
	// Close the HTTP server and database pool.
	testServer.Close()
	testPool.Close()

	// If we started a testcontainer, stop and remove it.
	// This is a no-op if we used an external database (containerCleanup is nil).
	if containerCleanup != nil {
		containerCleanup()
		fmt.Println("✓ testcontainers: PostgreSQL container stopped and removed")
	}

	os.Exit(exitCode)
}

// ---------------------------------------------------------------------------
// Database cleanup helpers
// ---------------------------------------------------------------------------

// truncateDataTables removes all rows from data tables (players, teams, etc.)
// while preserving reference tables (skills, gear_types) that contain static
// game data seeded by migrations.
//
// We use TRUNCATE ... CASCADE which also removes rows in tables that have
// foreign key references to the truncated tables. This is much faster than
// DELETE FROM and resets auto-increment sequences.
//
// WHY NOT truncate skills and gear_types?
//   Skills and gear types are reference data — they define what exists in the
//   game (e.g. "Shield Bash" costs 3 skill points). Tests need this data to
//   allocate skills and gear. Truncating them would break those tests.
func truncateDataTables(ctx context.Context) {
	// The order matters due to foreign key constraints, but CASCADE handles
	// dependent rows automatically. We list them explicitly for clarity.
	tables := []string{
		"sessions",
		"kredit_transactions",
		"arkade_point_logs",
		"leaderboard_entries",
		"player_skill_allocations",
		"player_gear",
		"join_requests",
		"team_members",
		"teams",
		"players",
	}

	for _, table := range tables {
		// fmt.Sprintf builds the SQL string dynamically.
		// Normally you should NEVER interpolate user input into SQL (SQL injection risk),
		// but here the table names are hardcoded constants, not user input, so it's safe.
		_, err := testPool.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: failed to truncate %s: %v\n", table, err)
		}
	}
}

// ---------------------------------------------------------------------------
// HTTP request helpers
// ---------------------------------------------------------------------------

// doRequest makes an HTTP request to the test server and returns the response.
//
// Parameters:
//   - method: HTTP method (GET, POST, PUT, DELETE)
//   - path:   URL path (e.g. "/auth/register") — appended to the test server's base URL
//   - body:   request body as a byte slice (nil for GET requests)
//   - token:  JWT token for authenticated requests (empty string for public endpoints)
//
// It returns the raw *http.Response so the caller can check status codes and read the body.
// The caller is responsible for closing resp.Body (typically via defer resp.Body.Close()).
func doRequest(t *testing.T, method, path string, body []byte, token string) *http.Response {
	// t.Helper() marks this function as a test helper.
	// When a test fails inside a helper, Go reports the line number of the
	// CALLER (the actual test), not the line inside this function.
	// This makes failure messages much easier to read.
	t.Helper()

	// Build the full URL by combining the test server's base URL with the path.
	url := testServer.URL + path

	// bytes.NewReader wraps a byte slice as an io.Reader, which http.NewRequest
	// expects for the request body. If body is nil (GET requests), we pass nil.
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	// http.NewRequest creates a new HTTP request object without sending it.
	// It returns (*http.Request, error).
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		// t.Fatalf stops the test immediately — there's no point continuing
		// if we can't even build the request.
		t.Fatalf("failed to create request %s %s: %v", method, path, err)
	}

	// Set the Content-Type header for requests that have a body (POST, PUT).
	// Our API expects JSON for all write operations.
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// If a JWT token was provided, add it as a Bearer token in the Authorization header.
	// This is how our auth middleware identifies the caller.
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// http.DefaultClient.Do sends the request and returns the response.
	// Unlike http.Get/http.Post, Do() gives us full control over method and headers.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request %s %s failed: %v", method, path, err)
	}

	return resp
}

// ---------------------------------------------------------------------------
// JSON parsing helpers
// ---------------------------------------------------------------------------

// parseJSON reads the response body and unmarshals it into a map[string]any.
//
// WHY map[string]any?
//   In Go, `any` is an alias for `interface{}` — it can hold any type.
//   map[string]any is Go's equivalent of a "generic JSON object" — it can
//   represent any JSON object without needing to define a struct.
//   This is useful in tests where we don't want to import internal types
//   and just need to check a few specific fields.
//
//   The downside is that all numbers become float64 (JSON's default number type),
//   so you need type assertions like `result["rank"].(float64)` to read them.
func parseJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()

	// io.ReadAll reads the entire response body into memory as a byte slice.
	// This is fine for test responses which are small.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	// Always close the body to free the underlying TCP connection.
	resp.Body.Close()

	// json.Unmarshal parses the JSON bytes into our target variable.
	// We pass a pointer (&result) because Unmarshal needs to write into it.
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to parse JSON response: %v\nBody was: %s", err, string(data))
	}

	return result
}

// parseJSONArray reads the response body and unmarshals it into a []any.
// Used for endpoints that return JSON arrays (e.g. GET /api/teams, GET /api/leaderboard).
func parseJSONArray(t *testing.T, resp *http.Response) []any {
	t.Helper()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	resp.Body.Close()

	var result []any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to parse JSON array response: %v\nBody was: %s", err, string(data))
	}

	return result
}

// readBody reads the full response body as a string.
// Used for non-JSON responses like HTML pages and the healthz endpoint.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	resp.Body.Close()

	return string(data)
}

// ---------------------------------------------------------------------------
// Assertion helpers
// ---------------------------------------------------------------------------

// assertStatus checks that the HTTP response has the expected status code.
// If it doesn't, it fails the test with a clear message showing what was expected vs actual.
func assertStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()

	if resp.StatusCode != expected {
		// Read the body to include in the error message — often the error response
		// contains useful information about why the status code was wrong.
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected status %d, got %d\nResponse body: %s", expected, resp.StatusCode, string(body))
	}
}

// assertContains checks that a string contains an expected substring.
// Used to verify HTML pages contain expected content (e.g. page titles, player names).
func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()

	if !strings.Contains(haystack, needle) {
		// Show a truncated version of haystack to avoid flooding the test output.
		preview := haystack
		if len(preview) > 500 {
			preview = preview[:500] + "... (truncated)"
		}
		t.Fatalf("expected response to contain %q, but it didn't.\nResponse preview: %s", needle, preview)
	}
}

// ---------------------------------------------------------------------------
// Auth convenience helpers
// ---------------------------------------------------------------------------

// registerPlayer creates a new player via POST /auth/register and returns
// the full AuthResponse as a map (contains "token" and "player" fields).
// This is used by many tests that need a logged-in player to test authenticated endpoints.
func registerPlayer(t *testing.T, username, email, password string) map[string]any {
	t.Helper()

	// json.Marshal converts a Go value to JSON bytes.
	// We use map[string]string here for simplicity — no need to define a struct
	// just to build a JSON body in a test.
	body, _ := json.Marshal(map[string]string{
		"username": username,
		"email":    email,
		"password": password,
	})

	resp := doRequest(t, "POST", "/auth/register", body, "")
	assertStatus(t, resp, http.StatusCreated)

	return parseJSON(t, resp)
}

// loginPlayer logs in via POST /auth/login and returns just the JWT token string.
// Many tests only need the token, not the full response.
func loginPlayer(t *testing.T, email, password string) string {
	t.Helper()

	body, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})

	resp := doRequest(t, "POST", "/auth/login", body, "")
	assertStatus(t, resp, http.StatusOK)

	result := parseJSON(t, resp)

	// Type assertion: result["token"] is stored as `any` (interface{}).
	// We need to assert it's actually a string. The .(string) syntax does this.
	// If it's not a string, this will panic — which is fine in tests because
	// it means the API response shape is wrong and we want to know immediately.
	return result["token"].(string)
}

// getPlayerID extracts the player's UUID from the "player" field of an AuthResponse.
// The response shape is: {"token": "...", "player": {"id": "uuid", ...}}
func getPlayerID(t *testing.T, authResp map[string]any) string {
	t.Helper()

	// authResp["player"] is a nested JSON object, which json.Unmarshal decodes
	// as map[string]any. We need two type assertions to navigate into it.
	player := authResp["player"].(map[string]any)
	return player["id"].(string)
}

// registerAndGetToken is a convenience that registers a player and returns
// both the token and player ID — the two things most tests need.
func registerAndGetToken(t *testing.T, username, email, password string) (token string, playerID string) {
	t.Helper()

	authResp := registerPlayer(t, username, email, password)
	token = authResp["token"].(string)
	playerID = getPlayerID(t, authResp)
	return token, playerID
}
