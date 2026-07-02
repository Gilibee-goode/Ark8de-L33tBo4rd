// Package tests — Leaderboard endpoint integration tests
//
// This file tests the leaderboard API:
//   GET /api/leaderboard            — all teams ranked by Arkade points
//   GET /api/leaderboard/teams/{id} — one team's leaderboard card
//
// HOW THE LEADERBOARD WORKS:
//   The leaderboard is backed by the `leaderboard_entries` table, which is a
//   denormalized cache updated whenever Arkade points change or a team is created.
//   It stores pre-computed fields (team name, tag, points, member count, rank)
//   so the leaderboard page can be rendered with a single fast query.
//
//   Ranks are calculated using SQL's RANK() window function, which assigns
//   equal rank to teams with the same points (e.g. two teams at 100 points
//   both get rank 1, and the next team gets rank 3, not 2).
package tests

import (
	// --- Standard library ---
	"encoding/json" // json.Marshal builds JSON request bodies
	"fmt"           // fmt.Sprintf builds URL paths with team IDs
	"net/http"      // HTTP status constants
	"testing"       // Go's test framework
)

// TestLeaderboard tests the leaderboard endpoints.
// It creates teams with different Arkade point totals and verifies the ranking.
func TestLeaderboard(t *testing.T) {
	// ---- Setup: create two teams and give them different Arkade points ----

	// Team 1: register owner, create team, award 100 points.
	owner1Token, _ := registerAndGetToken(t, "lb_owner1", "lb1@test.dev", "LbOwner1!")
	body, _ := json.Marshal(map[string]string{"name": "LB Team One", "tag": "LB01"})
	resp := doRequest(t, "POST", "/api/teams", body, owner1Token)
	assertStatus(t, resp, http.StatusCreated)
	team1 := parseJSON(t, resp)
	team1ID := team1["id"].(string)

	// Team 2: register owner, create team, award 50 points.
	owner2Token, _ := registerAndGetToken(t, "lb_owner2", "lb2@test.dev", "LbOwner2!")
	body, _ = json.Marshal(map[string]string{"name": "LB Team Two", "tag": "LB02"})
	resp = doRequest(t, "POST", "/api/teams", body, owner2Token)
	assertStatus(t, resp, http.StatusCreated)
	team2 := parseJSON(t, resp)
	team2ID := team2["id"].(string)

	// Create a moderator to award points.
	modToken, _ := registerAndGetToken(t, "lb_mod", "lb_mod@test.dev", "LbModPass1!")
	_, err := testPool.Exec(contextBg(), "UPDATE players SET role = 'moderator' WHERE username = 'lb_mod'")
	if err != nil {
		t.Fatalf("failed to promote moderator: %v", err)
	}
	modToken = loginPlayer(t, "lb_mod@test.dev", "LbModPass1!")

	// Award 100 points to Team 1.
	body, _ = json.Marshal(map[string]any{"delta": 100, "reason": "Test"})
	resp = doRequest(t, "PUT", fmt.Sprintf("/api/teams/%s/points", team1ID), body, modToken)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Award 50 points to Team 2.
	body, _ = json.Marshal(map[string]any{"delta": 50, "reason": "Test"})
	resp = doRequest(t, "PUT", fmt.Sprintf("/api/teams/%s/points", team2ID), body, modToken)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// ---- Subtest: GET /api/leaderboard returns ranked results ----
	t.Run("full_leaderboard", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/leaderboard", nil, "")
		assertStatus(t, resp, http.StatusOK)

		entries := parseJSONArray(t, resp)
		if len(entries) < 2 {
			t.Fatalf("expected at least 2 leaderboard entries, got %d", len(entries))
		}

		// The first entry should be the team with the most points (Team 1 = 100).
		first := entries[0].(map[string]any)
		if first["team_name"] != "LB Team One" {
			t.Fatalf("expected 'LB Team One' at rank 1, got %v", first["team_name"])
		}
		if first["arkade_points"].(float64) != 100 {
			t.Fatalf("expected 100 arkade points for rank 1, got %v", first["arkade_points"])
		}
	})

	// ---- Subtest: GET /api/leaderboard/teams/{id} returns a single team's card ----
	t.Run("team_card", func(t *testing.T) {
		resp := doRequest(t, "GET", fmt.Sprintf("/api/leaderboard/teams/%s", team2ID), nil, "")
		assertStatus(t, resp, http.StatusOK)

		result := parseJSON(t, resp)
		if result["team_name"] != "LB Team Two" {
			t.Fatalf("expected 'LB Team Two', got %v", result["team_name"])
		}
		if result["arkade_points"].(float64) != 50 {
			t.Fatalf("expected 50 arkade points, got %v", result["arkade_points"])
		}
	})

	// ---- Subtest: nonexistent team returns 404 ----
	t.Run("not_found", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/leaderboard/teams/00000000-0000-0000-0000-000000000000", nil, "")
		assertStatus(t, resp, http.StatusNotFound)
	})

	// ---- Subtest: convenience — make sure tokens/owners aren't mixed up ----
	_ = owner1Token
	_ = owner2Token
}
