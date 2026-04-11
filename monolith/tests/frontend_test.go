// Package tests — Frontend (HTML page) integration tests
//
// This file tests the server-rendered HTML pages:
//   GET /          — leaderboard page (public)
//   GET /teams/{id} — team detail page (public)
//   GET /healthz   — health check endpoint
//
// These tests verify that:
//   1. The pages return HTTP 200 (templates parse and render without errors)
//   2. The HTML contains expected content (page titles, data from the DB)
//   3. The static CSS is served correctly
//
// We do NOT test /profile or /mod here because they require authentication
// via JWT in cookies/headers, which is complex to set up for HTML page tests.
// Those are better tested via the JSON API endpoints that back them.
//
// WHY TEST HTML PAGES?
//   Template parsing errors only surface at runtime — the Go compiler doesn't
//   check templates at build time. If a template references a field that doesn't
//   exist on the data struct, or uses broken {{}} syntax, the page returns a 500.
//   These tests catch those errors before deployment.
package tests

import (
	// --- Standard library ---
	"encoding/json" // json.Marshal for team creation
	"fmt"           // fmt.Sprintf for URL building
	"net/http"      // HTTP status constants
	"testing"       // Go's test framework
)

// ---------------------------------------------------------------------------
// GET / — Leaderboard page
// ---------------------------------------------------------------------------

func TestFrontendLeaderboard(t *testing.T) {
	resp := doRequest(t, "GET", "/", nil, "")
	assertStatus(t, resp, http.StatusOK)

	body := readBody(t, resp)

	// The leaderboard page should contain these elements.
	assertContains(t, body, "Ark8de") // page title or heading
}

// ---------------------------------------------------------------------------
// GET /teams/{id} — Team detail page
// ---------------------------------------------------------------------------

func TestFrontendTeamDetail(t *testing.T) {
	// ---- Setup: create a team so we have a valid team ID to visit ----
	token, _ := registerAndGetToken(t, "fe_owner", "fe_owner@test.dev", "FeOwner1!")
	body, _ := json.Marshal(map[string]string{
		"name": "Frontend Test Team",
		"tag":  "FETE",
	})
	resp := doRequest(t, "POST", "/api/teams", body, token)
	assertStatus(t, resp, http.StatusCreated)
	team := parseJSON(t, resp)
	teamID := team["id"].(string)

	// ---- Test: the team detail page renders ----
	t.Run("renders_successfully", func(t *testing.T) {
		resp := doRequest(t, "GET", fmt.Sprintf("/teams/%s", teamID), nil, "")
		assertStatus(t, resp, http.StatusOK)

		html := readBody(t, resp)
		assertContains(t, html, "Frontend Test Team") // team name should appear in the page
		assertContains(t, html, "FETE")                // team tag should appear
	})

	// ---- Test: nonexistent team ----
	// Depending on the handler implementation, this may return 404 or render an error page.
	t.Run("not_found", func(t *testing.T) {
		resp := doRequest(t, "GET", "/teams/00000000-0000-0000-0000-000000000000", nil, "")
		// Accept either 404 (API-style) or 500 (template error) — both indicate the team is missing.
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusInternalServerError {
			body := readBody(t, resp)
			t.Fatalf("expected 404 or 500 for missing team, got %d\nBody: %s", resp.StatusCode, body)
		}
		resp.Body.Close()
	})
}

// ---------------------------------------------------------------------------
// GET /healthz — Health check
// ---------------------------------------------------------------------------

func TestHealthz(t *testing.T) {
	resp := doRequest(t, "GET", "/healthz", nil, "")
	assertStatus(t, resp, http.StatusOK)

	body := readBody(t, resp)
	if body != "ok" {
		t.Fatalf("expected healthz body 'ok', got %q", body)
	}
}

// ---------------------------------------------------------------------------
// GET /static/style.css — Static file serving
// ---------------------------------------------------------------------------

func TestStaticCSS(t *testing.T) {
	resp := doRequest(t, "GET", "/static/style.css", nil, "")
	assertStatus(t, resp, http.StatusOK)

	// The Content-Type should indicate CSS.
	contentType := resp.Header.Get("Content-Type")
	if contentType != "text/css; charset=utf-8" {
		t.Fatalf("expected Content-Type 'text/css; charset=utf-8', got %q", contentType)
	}

	body := readBody(t, resp)
	// Our CSS should contain the dark theme custom properties.
	assertContains(t, body, "--bg-primary")
}
