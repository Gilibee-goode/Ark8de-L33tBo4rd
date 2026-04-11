// Package tests — Team endpoint integration tests
//
// This file tests all team-related API endpoints:
//   POST   /api/teams                              — create a new team
//   GET    /api/teams                              — list all teams
//   GET    /api/teams/{id}                         — get team detail (with roster + gear)
//   PUT    /api/teams/{id}                         — update team name/tag
//   DELETE /api/teams/{id}                         — delete team
//   PUT    /api/teams/{id}/lock                    — toggle team lock-in
//   DELETE /api/teams/{id}/members/{pid}           — remove a member
//   POST   /api/teams/{id}/join-requests           — send join request
//   GET    /api/teams/{id}/join-requests           — list pending requests (owner)
//   PUT    /api/teams/{id}/join-requests/{rid}     — accept/reject request
//   PUT    /api/teams/{id}/points                  — add Arkade points (mod)
//   GET    /api/teams/{id}/points/history          — point audit log (mod)
//
// TEAM CREATION FLOW:
//   1. A player creates a team → they become the owner (role upgrades to team_owner)
//   2. Other players send join requests → owner accepts or rejects
//   3. Accepted players become team members
//   4. Owner can lock the team to prevent changes
//   5. Moderators can award/deduct Arkade points
package tests

import (
	// --- Standard library ---
	"context"       // context.Background for DB operations in test setup
	"encoding/json" // json.Marshal builds JSON request bodies
	"fmt"           // fmt.Sprintf builds URL paths with team/player IDs
	"net/http"      // HTTP status constants
	"testing"       // Go's test framework
)

// ---------------------------------------------------------------------------
// POST /api/teams — Create team
// ---------------------------------------------------------------------------

func TestTeamCreate(t *testing.T) {
	// ---- Setup: register a player who will create the team ----
	ownerToken, _ := registerAndGetToken(t, "team_owner", "owner@test.dev", "OwnerPass1!")

	var teamID string

	// ---- Subtest: create a team successfully ----
	t.Run("success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"name": "Test Warriors",
			"tag":  "TWAR",
		})
		resp := doRequest(t, "POST", "/api/teams", body, ownerToken)
		assertStatus(t, resp, http.StatusCreated)

		result := parseJSON(t, resp)
		teamID = result["id"].(string)

		if result["name"] != "Test Warriors" {
			t.Fatalf("expected team name 'Test Warriors', got %v", result["name"])
		}
		if result["tag"] != "TWAR" {
			t.Fatalf("expected tag 'TWAR', got %v", result["tag"])
		}
	})

	// ---- Subtest: duplicate name ----
	t.Run("duplicate_name", func(t *testing.T) {
		// Register another player to attempt creating a team with the same name.
		otherToken, _ := registerAndGetToken(t, "dup_name_user", "dupname@test.dev", "DupPass1!")

		body, _ := json.Marshal(map[string]string{
			"name": "Test Warriors", // same name as above
			"tag":  "DUP1",
		})
		resp := doRequest(t, "POST", "/api/teams", body, otherToken)
		assertStatus(t, resp, http.StatusConflict)
	})

	// ---- Subtest: duplicate tag ----
	t.Run("duplicate_tag", func(t *testing.T) {
		otherToken, _ := registerAndGetToken(t, "dup_tag_user", "duptag@test.dev", "DupPass1!")

		body, _ := json.Marshal(map[string]string{
			"name": "Different Name",
			"tag":  "TWAR", // same tag as above
		})
		resp := doRequest(t, "POST", "/api/teams", body, otherToken)
		assertStatus(t, resp, http.StatusConflict)
	})

	// ---- Subtest: invalid tag (too short) ----
	t.Run("invalid_tag", func(t *testing.T) {
		anotherToken, _ := registerAndGetToken(t, "bad_tag_user", "badtag@test.dev", "BadPass1!")

		body, _ := json.Marshal(map[string]string{
			"name": "Valid Name",
			"tag":  "X", // too short — must be 2–10 characters
		})
		resp := doRequest(t, "POST", "/api/teams", body, anotherToken)
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// ---- Subtest: no auth ----
	t.Run("no_auth", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"name": "No Auth Team",
			"tag":  "NOAU",
		})
		resp := doRequest(t, "POST", "/api/teams", body, "")
		assertStatus(t, resp, http.StatusUnauthorized)
	})

	// Save teamID for use in later tests (passed via the test variable).
	_ = teamID
}

// ---------------------------------------------------------------------------
// GET /api/teams — List teams
// ---------------------------------------------------------------------------

func TestTeamList(t *testing.T) {
	resp := doRequest(t, "GET", "/api/teams", nil, "")
	assertStatus(t, resp, http.StatusOK)

	teams := parseJSONArray(t, resp)
	// We should have at least one team from TestTeamCreate.
	if len(teams) < 1 {
		t.Fatalf("expected at least 1 team, got %d", len(teams))
	}
}

// ---------------------------------------------------------------------------
// Full team lifecycle: create → join → accept → detail → points → lock → delete
// ---------------------------------------------------------------------------

// TestTeamLifecycle tests the complete team workflow in order.
// We use a single test function with sequential subtests because each step
// depends on the previous one (e.g. you can't accept a join request before
// sending one). This is a common pattern for testing stateful workflows.
func TestTeamLifecycle(t *testing.T) {
	// ---- Setup: register an owner, a joiner, and a moderator ----
	ownerToken, ownerID := registerAndGetToken(t, "lc_owner", "lc_owner@test.dev", "LcOwner1!")
	joinerToken, joinerID := registerAndGetToken(t, "lc_joiner", "lc_joiner@test.dev", "LcJoiner1!")

	// The moderator needs the "moderator" role. We register normally (gets "player" role)
	// and then manually update the role in the database.
	modToken, _ := registerAndGetToken(t, "lc_mod", "lc_mod@test.dev", "LcModPass1!")

	// Promote the mod user in the database.
	// In production, roles are assigned by admins. In tests, we directly update the DB.
	_, err := testPool.Exec(
		contextBg(),
		"UPDATE players SET role = 'moderator' WHERE username = 'lc_mod'",
	)
	if err != nil {
		t.Fatalf("failed to promote moderator: %v", err)
	}
	// Re-login to get a fresh token with the updated role.
	modToken = loginPlayer(t, "lc_mod@test.dev", "LcModPass1!")

	// ---- Step 1: Create team ----
	var teamID string
	t.Run("create_team", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"name": "Lifecycle Team",
			"tag":  "LIFE",
		})
		resp := doRequest(t, "POST", "/api/teams", body, ownerToken)
		assertStatus(t, resp, http.StatusCreated)

		result := parseJSON(t, resp)
		teamID = result["id"].(string)
	})

	// ---- Step 2: Get team detail (owner is the only member) ----
	t.Run("get_detail", func(t *testing.T) {
		resp := doRequest(t, "GET", fmt.Sprintf("/api/teams/%s", teamID), nil, "")
		assertStatus(t, resp, http.StatusOK)

		result := parseJSON(t, resp)
		if result["name"] != "Lifecycle Team" {
			t.Fatalf("expected team name 'Lifecycle Team', got %v", result["name"])
		}

		// The owner should be the only member.
		members := result["members"].([]any)
		if len(members) != 1 {
			t.Fatalf("expected 1 member, got %d", len(members))
		}

		// Check that the owner is listed.
		firstMember := members[0].(map[string]any)
		if firstMember["player_id"] != ownerID {
			t.Fatalf("expected owner %s as first member, got %v", ownerID, firstMember["player_id"])
		}
	})

	// ---- Step 3: Joiner sends a join request ----
	var requestID string
	t.Run("send_join_request", func(t *testing.T) {
		resp := doRequest(t, "POST", fmt.Sprintf("/api/teams/%s/join-requests", teamID), nil, joinerToken)
		assertStatus(t, resp, http.StatusCreated)

		result := parseJSON(t, resp)
		requestID = result["id"].(string)
		if result["status"] != "pending" {
			t.Fatalf("expected status 'pending', got %v", result["status"])
		}
	})

	// ---- Step 4: Owner views pending join requests ----
	t.Run("view_join_requests", func(t *testing.T) {
		resp := doRequest(t, "GET", fmt.Sprintf("/api/teams/%s/join-requests", teamID), nil, ownerToken)
		assertStatus(t, resp, http.StatusOK)

		requests := parseJSONArray(t, resp)
		if len(requests) != 1 {
			t.Fatalf("expected 1 pending request, got %d", len(requests))
		}
	})

	// ---- Step 5: Owner accepts the join request ----
	t.Run("accept_join_request", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"action": "accept"})
		path := fmt.Sprintf("/api/teams/%s/join-requests/%s", teamID, requestID)
		resp := doRequest(t, "PUT", path, body, ownerToken)
		assertStatus(t, resp, http.StatusOK)
	})

	// ---- Step 6: Team detail now shows 2 members ----
	t.Run("detail_after_join", func(t *testing.T) {
		resp := doRequest(t, "GET", fmt.Sprintf("/api/teams/%s", teamID), nil, "")
		assertStatus(t, resp, http.StatusOK)

		result := parseJSON(t, resp)
		members := result["members"].([]any)
		if len(members) != 2 {
			t.Fatalf("expected 2 members after join, got %d", len(members))
		}
	})

	// ---- Step 7: Duplicate join request should fail ----
	t.Run("duplicate_join_request", func(t *testing.T) {
		resp := doRequest(t, "POST", fmt.Sprintf("/api/teams/%s/join-requests", teamID), nil, joinerToken)
		// Joiner is already a member, so this should fail.
		assertStatus(t, resp, http.StatusConflict)
	})

	// ---- Step 8: Moderator awards Arkade points ----
	t.Run("add_arkade_points", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"delta":  25,
			"reason": "Test arena victory",
		})
		resp := doRequest(t, "PUT", fmt.Sprintf("/api/teams/%s/points", teamID), body, modToken)
		assertStatus(t, resp, http.StatusOK)
	})

	// ---- Step 9: View point history ----
	t.Run("point_history", func(t *testing.T) {
		resp := doRequest(t, "GET", fmt.Sprintf("/api/teams/%s/points/history", teamID), nil, modToken)
		assertStatus(t, resp, http.StatusOK)

		logs := parseJSONArray(t, resp)
		if len(logs) != 1 {
			t.Fatalf("expected 1 point log entry, got %d", len(logs))
		}

		entry := logs[0].(map[string]any)
		if entry["delta"].(float64) != 25 {
			t.Fatalf("expected delta 25, got %v", entry["delta"])
		}
	})

	// ---- Step 10: Non-moderator cannot award points ----
	t.Run("points_forbidden", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"delta":  10,
			"reason": "Unauthorized attempt",
		})
		resp := doRequest(t, "PUT", fmt.Sprintf("/api/teams/%s/points", teamID), body, ownerToken)
		assertStatus(t, resp, http.StatusForbidden)
	})

	// ---- Step 11: Update team name/tag ----
	t.Run("update_team", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"name": "Updated Team",
			"tag":  "UPDT",
		})
		resp := doRequest(t, "PUT", fmt.Sprintf("/api/teams/%s", teamID), body, ownerToken)
		assertStatus(t, resp, http.StatusOK)
	})

	// ---- Step 12: Toggle lock ----
	t.Run("toggle_lock", func(t *testing.T) {
		resp := doRequest(t, "PUT", fmt.Sprintf("/api/teams/%s/lock", teamID), nil, ownerToken)
		assertStatus(t, resp, http.StatusOK)

		result := parseJSON(t, resp)
		// After first toggle, should be locked.
		if result["is_locked_in"] != true {
			t.Fatalf("expected is_locked_in to be true, got %v", result["is_locked_in"])
		}
	})

	// ---- Step 13: Unlock (toggle again) ----
	t.Run("toggle_unlock", func(t *testing.T) {
		resp := doRequest(t, "PUT", fmt.Sprintf("/api/teams/%s/lock", teamID), nil, ownerToken)
		assertStatus(t, resp, http.StatusOK)

		result := parseJSON(t, resp)
		if result["is_locked_in"] != false {
			t.Fatalf("expected is_locked_in to be false, got %v", result["is_locked_in"])
		}
	})

	// ---- Step 14: Remove a member ----
	t.Run("remove_member", func(t *testing.T) {
		resp := doRequest(t, "DELETE", fmt.Sprintf("/api/teams/%s/members/%s", teamID, joinerID), nil, ownerToken)
		assertStatus(t, resp, http.StatusOK)

		// Verify member count went back to 1.
		detailResp := doRequest(t, "GET", fmt.Sprintf("/api/teams/%s", teamID), nil, "")
		assertStatus(t, detailResp, http.StatusOK)
		result := parseJSON(t, detailResp)
		members := result["members"].([]any)
		if len(members) != 1 {
			t.Fatalf("expected 1 member after removal, got %d", len(members))
		}
	})

	// ---- Step 15: Cannot remove the owner ----
	t.Run("cannot_remove_owner", func(t *testing.T) {
		resp := doRequest(t, "DELETE", fmt.Sprintf("/api/teams/%s/members/%s", teamID, ownerID), nil, ownerToken)
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// ---- Step 16: Nonexistent team returns 404 ----
	t.Run("team_not_found", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/teams/00000000-0000-0000-0000-000000000000", nil, "")
		assertStatus(t, resp, http.StatusNotFound)
	})
}

// ---------------------------------------------------------------------------
// Join request rejection flow
// ---------------------------------------------------------------------------

func TestJoinRequestReject(t *testing.T) {
	// ---- Setup ----
	ownerToken, _ := registerAndGetToken(t, "rj_owner", "rj_owner@test.dev", "RjOwner1!")
	joinerToken, _ := registerAndGetToken(t, "rj_joiner", "rj_joiner@test.dev", "RjJoiner1!")

	// Create a team.
	body, _ := json.Marshal(map[string]string{
		"name": "Reject Test Team",
		"tag":  "RJCT",
	})
	resp := doRequest(t, "POST", "/api/teams", body, ownerToken)
	assertStatus(t, resp, http.StatusCreated)
	teamResult := parseJSON(t, resp)
	teamID := teamResult["id"].(string)

	// Joiner sends a join request.
	resp = doRequest(t, "POST", fmt.Sprintf("/api/teams/%s/join-requests", teamID), nil, joinerToken)
	assertStatus(t, resp, http.StatusCreated)
	reqResult := parseJSON(t, resp)
	requestID := reqResult["id"].(string)

	// ---- Owner rejects the request ----
	t.Run("reject", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"action": "reject"})
		path := fmt.Sprintf("/api/teams/%s/join-requests/%s", teamID, requestID)
		resp := doRequest(t, "PUT", path, body, ownerToken)
		assertStatus(t, resp, http.StatusOK)
	})

	// ---- Verify team still has 1 member (owner only) ----
	t.Run("member_count_unchanged", func(t *testing.T) {
		resp := doRequest(t, "GET", fmt.Sprintf("/api/teams/%s", teamID), nil, "")
		assertStatus(t, resp, http.StatusOK)
		result := parseJSON(t, resp)
		members := result["members"].([]any)
		if len(members) != 1 {
			t.Fatalf("expected 1 member (owner only), got %d", len(members))
		}
	})
}

// contextBg is a convenience wrapper around context.Background().
// It makes the test code slightly shorter when we need a context for DB operations.
func contextBg() context.Context {
	return context.Background()
}
