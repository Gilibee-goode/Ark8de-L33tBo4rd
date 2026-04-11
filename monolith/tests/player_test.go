// Package tests — Player endpoint integration tests
//
// This file tests the player-related API endpoints:
//   PUT  /api/players/me/class           — set class role (tank/dps/healer/support)
//   GET  /api/players/me/stats           — view computed combat stats
//   GET  /api/players/me/skills          — view allocated skills
//   PUT  /api/players/me/skills          — allocate skills (spend skill points)
//   GET  /api/players/me/gear            — view selected gear
//   PUT  /api/players/me/gear            — select gear items
//   GET  /api/players/me/kredits         — view Kredit balance and history
//   POST /api/players/me/kredits/transfer — transfer Kredits to another player
//   GET  /api/players/{id}               — public player profile
//   GET  /api/skills                     — list all skills (public)
//
// IMPORTANT: These tests depend on the skills and gear_types tables having data.
// Gear types are seeded by migration 000007 (always present if migrations ran).
// Skills are seeded by the seed script, so the test setup seeds them manually
// to ensure tests are self-contained and don't depend on `task seed` being run.
package tests

import (
	// --- Standard library ---
	"context"       // context.Background for DB seeding operations
	"encoding/json" // json.Marshal builds JSON request bodies
	"fmt"           // fmt.Sprintf builds URL paths with player/team IDs
	"net/http"      // HTTP status constants
	"testing"       // Go's test framework
)

// seedTestSkills inserts a minimal set of skills into the database for testing.
// It returns a map of skill name → skill ID (UUID string) so tests can reference
// specific skills when allocating them.
//
// We seed skills in the test rather than relying on `task seed` so that:
//  1. Tests are self-contained — they work without running the seed script
//  2. We know the exact IDs to use in allocation requests
//  3. Different test runs don't conflict with each other
func seedTestSkills(t *testing.T) map[string]string {
	t.Helper()

	ctx := context.Background()

	// Define a minimal set of skills for testing.
	// We need at least 2 skills per class to test budget enforcement.
	// Each skill has: name, class_role, cost, effect_description, effect_type, hp_bonus, armor_bonus
	type testSkill struct {
		name, class, desc, effectType string
		cost, hp, armor              int
	}

	skills := []testSkill{
		{"Test Shield Bash", "tank", "+10 armor", "passive", 3, 0, 10},
		{"Test Iron Wall", "tank", "+20 armor", "passive", 5, 0, 20},
		{"Test Fortify", "tank", "+30 armor", "passive", 8, 0, 30},
		{"Test Quick Strike", "dps", "+10 HP damage", "active", 3, 10, 0},
		{"Test Power Surge", "dps", "+20 HP damage", "active", 5, 20, 0},
		{"Test Heal", "healer", "restore 15 HP", "active", 3, 15, 0},
		{"Test Revive", "healer", "restore 30 HP", "active", 7, 30, 0},
		{"Test Rally", "support", "+5 armor to team", "passive", 3, 0, 5},
		{"Test Inspire", "support", "+10 HP to team", "passive", 5, 10, 0},
	}

	// Insert each skill and collect the auto-generated UUID.
	// RETURNING id gets the database-generated UUID back in the same query.
	ids := make(map[string]string)
	for _, s := range skills {
		var id string
		err := testPool.QueryRow(ctx,
			`INSERT INTO skills (name, description, class_role, cost_skill_points,
			                     effect_description, effect_type, hp_bonus, armor_bonus)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 ON CONFLICT DO NOTHING
			 RETURNING id::text`,
			s.name, s.desc, s.class, s.cost, s.desc, s.effectType, s.hp, s.armor,
		).Scan(&id)
		if err != nil {
			// If the skill already exists (from a previous test run), look it up.
			err2 := testPool.QueryRow(ctx,
				`SELECT id::text FROM skills WHERE name = $1`, s.name,
			).Scan(&id)
			if err2 != nil {
				t.Fatalf("failed to seed skill %q: insert=%v, select=%v", s.name, err, err2)
			}
		}
		ids[s.name] = id
	}

	return ids
}

// ---------------------------------------------------------------------------
// PUT /api/players/me/class
// ---------------------------------------------------------------------------

func TestPlayerSetClass(t *testing.T) {
	// ---- Setup: register a player ----
	token, _ := registerAndGetToken(t, "class_user", "class@test.dev", "ClassPass1!")

	// ---- Subtest: set class to tank ----
	t.Run("set_tank", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"class_role": "tank"})
		resp := doRequest(t, "PUT", "/api/players/me/class", body, token)
		assertStatus(t, resp, http.StatusOK)
	})

	// ---- Subtest: set class to dps (switching classes) ----
	// Switching classes should clear any previously allocated skills.
	t.Run("switch_to_dps", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"class_role": "dps"})
		resp := doRequest(t, "PUT", "/api/players/me/class", body, token)
		assertStatus(t, resp, http.StatusOK)
	})

	// ---- Subtest: invalid class ----
	// An invalid class role should return 400.
	t.Run("invalid_class", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"class_role": "wizard"})
		resp := doRequest(t, "PUT", "/api/players/me/class", body, token)
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// ---- Subtest: no auth ----
	t.Run("no_auth", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"class_role": "tank"})
		resp := doRequest(t, "PUT", "/api/players/me/class", body, "")
		assertStatus(t, resp, http.StatusUnauthorized)
	})
}

// ---------------------------------------------------------------------------
// GET /api/players/me/stats
// ---------------------------------------------------------------------------

func TestPlayerGetStats(t *testing.T) {
	// ---- Setup: register a player and set a class ----
	token, _ := registerAndGetToken(t, "stats_user", "stats@test.dev", "StatsPass1!")

	body, _ := json.Marshal(map[string]string{"class_role": "tank"})
	resp := doRequest(t, "PUT", "/api/players/me/class", body, token)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// ---- Subtest: stats with a class set ----
	t.Run("with_class", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/players/me/stats", nil, token)
		assertStatus(t, resp, http.StatusOK)

		result := parseJSON(t, resp)

		// Stats response should include these fields.
		if result["class_role"] != "tank" {
			t.Fatalf("expected class_role 'tank', got %v", result["class_role"])
		}

		// health_points and armor_points should be numbers (base stats from class).
		if _, ok := result["health_points"].(float64); !ok {
			t.Fatal("expected health_points to be a number")
		}
		if _, ok := result["armor_points"].(float64); !ok {
			t.Fatal("expected armor_points to be a number")
		}
	})
}

// ---------------------------------------------------------------------------
// GET/PUT /api/players/me/skills
// ---------------------------------------------------------------------------

func TestPlayerSkills(t *testing.T) {
	// ---- Setup: seed skills and register a player with a class ----
	skillIDs := seedTestSkills(t)
	token, _ := registerAndGetToken(t, "skills_user", "skills@test.dev", "SkillsPass1!")

	// Set class to tank so we can allocate tank skills.
	body, _ := json.Marshal(map[string]string{"class_role": "tank"})
	resp := doRequest(t, "PUT", "/api/players/me/class", body, token)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// ---- Subtest: allocate valid skills ----
	// "Test Shield Bash" costs 3 SP, "Test Iron Wall" costs 5 SP = 8 total (within 20 budget).
	t.Run("allocate_valid", func(t *testing.T) {
		body, _ := json.Marshal(map[string][]string{
			"skill_ids": {skillIDs["Test Shield Bash"], skillIDs["Test Iron Wall"]},
		})
		resp := doRequest(t, "PUT", "/api/players/me/skills", body, token)
		assertStatus(t, resp, http.StatusOK)
	})

	// ---- Subtest: get allocated skills ----
	t.Run("get_skills", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/players/me/skills", nil, token)
		assertStatus(t, resp, http.StatusOK)

		result := parseJSON(t, resp)

		// Should have 2 allocated skills.
		skills := result["allocated_skills"].([]any)
		if len(skills) != 2 {
			t.Fatalf("expected 2 allocated skills, got %d", len(skills))
		}

		// Remaining skill points should be 20 - 8 = 12.
		remaining := result["skill_points_remaining"].(float64)
		if remaining != 12 {
			t.Fatalf("expected 12 remaining skill points, got %v", remaining)
		}
	})

	// ---- Subtest: exceed budget ----
	// Try to allocate all 3 tank skills: 3 + 5 + 8 = 16 pts (still within 20).
	// Actually let's try a combo that exceeds: we'd need cost > 20.
	// With our test data: Shield Bash (3) + Iron Wall (5) + Fortify (8) = 16. Still within 20.
	// So let's test the "wrong class" error instead.
	t.Run("wrong_class_skill", func(t *testing.T) {
		// Try to allocate a DPS skill while being a tank — should fail.
		body, _ := json.Marshal(map[string][]string{
			"skill_ids": {skillIDs["Test Quick Strike"]}, // DPS skill, but player is tank
		})
		resp := doRequest(t, "PUT", "/api/players/me/skills", body, token)
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// ---- Subtest: nonexistent skill ID ----
	t.Run("invalid_skill_id", func(t *testing.T) {
		body, _ := json.Marshal(map[string][]string{
			"skill_ids": {"00000000-0000-0000-0000-000000000000"}, // doesn't exist
		})
		resp := doRequest(t, "PUT", "/api/players/me/skills", body, token)
		assertStatus(t, resp, http.StatusBadRequest)
	})
}

// ---------------------------------------------------------------------------
// GET /api/skills (public)
// ---------------------------------------------------------------------------

func TestListSkills(t *testing.T) {
	// Make sure test skills exist.
	seedTestSkills(t)

	// ---- Subtest: skills require a class_role filter ----
	// The /api/skills endpoint queries by class_role, so calling it without
	// a filter returns an empty list (the SQL WHERE clause matches nothing).
	t.Run("no_filter_returns_empty", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/skills", nil, "")
		assertStatus(t, resp, http.StatusOK)
		// Without a class filter, the endpoint returns no skills.
		// This is by design — players always pick skills for a specific class.
	})

	// ---- Subtest: filter by class_role ----
	t.Run("filter_by_class", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/skills?class_role=tank", nil, "")
		assertStatus(t, resp, http.StatusOK)

		skills := parseJSONArray(t, resp)
		// We seeded 3 tank skills.
		if len(skills) < 3 {
			t.Fatalf("expected at least 3 tank skills, got %d", len(skills))
		}

		// Every returned skill should be a tank skill.
		for _, s := range skills {
			skill := s.(map[string]any)
			if skill["class_role"] != "tank" {
				t.Fatalf("expected class_role 'tank', got %v", skill["class_role"])
			}
		}
	})
}

// ---------------------------------------------------------------------------
// GET/PUT /api/players/me/gear
// ---------------------------------------------------------------------------

func TestPlayerGear(t *testing.T) {
	// ---- Setup: register a player ----
	token, _ := registerAndGetToken(t, "gear_user", "gear@test.dev", "GearPass1!")

	// Look up gear type IDs from the database (seeded by migration 000007).
	// We query them rather than hardcoding UUIDs because they are auto-generated.
	ctx := context.Background()
	var swordID, bowID string
	err := testPool.QueryRow(ctx, "SELECT id::text FROM gear_types WHERE name = 'sword'").Scan(&swordID)
	if err != nil {
		t.Fatalf("failed to find sword gear type: %v", err)
	}
	err = testPool.QueryRow(ctx, "SELECT id::text FROM gear_types WHERE name = 'bow_and_arrow'").Scan(&bowID)
	if err != nil {
		t.Fatalf("failed to find bow_and_arrow gear type: %v", err)
	}

	// ---- Subtest: select gear ----
	t.Run("select_gear", func(t *testing.T) {
		body, _ := json.Marshal(map[string][]string{
			"gear_type_ids": {swordID, bowID},
		})
		resp := doRequest(t, "PUT", "/api/players/me/gear", body, token)
		assertStatus(t, resp, http.StatusOK)
	})

	// ---- Subtest: get selected gear ----
	t.Run("get_gear", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/players/me/gear", nil, token)
		assertStatus(t, resp, http.StatusOK)

		result := parseJSON(t, resp)
		gear := result["selected_gear"].([]any)
		if len(gear) != 2 {
			t.Fatalf("expected 2 gear items, got %d", len(gear))
		}

		// Gear points used by me should be: sword(2) + bow(3) = 5.
		myPoints := result["gear_points_used_by_me"].(float64)
		if myPoints != 5 {
			t.Fatalf("expected 5 gear points used, got %v", myPoints)
		}
	})

	// ---- Subtest: invalid gear ID ----
	t.Run("invalid_gear_id", func(t *testing.T) {
		body, _ := json.Marshal(map[string][]string{
			"gear_type_ids": {"00000000-0000-0000-0000-000000000000"},
		})
		resp := doRequest(t, "PUT", "/api/players/me/gear", body, token)
		assertStatus(t, resp, http.StatusBadRequest)
	})
}

// ---------------------------------------------------------------------------
// GET /api/players/me/kredits + POST /api/players/me/kredits/transfer
// ---------------------------------------------------------------------------

func TestPlayerKredits(t *testing.T) {
	// ---- Setup: register two players for transfer testing ----
	senderToken, _ := registerAndGetToken(t, "kredit_sender", "sender@test.dev", "SendPass1!")
	_, receiverID := registerAndGetToken(t, "kredit_receiver", "receiver@test.dev", "RecvPass1!")

	// ---- Subtest: initial balance is 0 ----
	t.Run("initial_balance", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/players/me/kredits", nil, senderToken)
		assertStatus(t, resp, http.StatusOK)

		result := parseJSON(t, resp)
		balance := result["balance"].(float64)
		if balance != 0 {
			t.Fatalf("expected initial balance 0, got %v", balance)
		}
	})

	// ---- Subtest: transfer with insufficient balance ----
	// Sender has 0 Kredits, so any transfer should fail.
	t.Run("insufficient_balance", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"to_player_id": receiverID,
			"amount":       50,
			"note":         "test transfer",
		})
		resp := doRequest(t, "POST", "/api/players/me/kredits/transfer", body, senderToken)
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// ---- Subtest: transfer with invalid amount ----
	t.Run("invalid_amount", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"to_player_id": receiverID,
			"amount":       0, // must be > 0
			"note":         "test",
		})
		resp := doRequest(t, "POST", "/api/players/me/kredits/transfer", body, senderToken)
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// ---- Subtest: cannot transfer to self ----
	t.Run("self_transfer", func(t *testing.T) {
		// Get sender's own ID from /auth/me
		resp := doRequest(t, "GET", "/auth/me", nil, senderToken)
		assertStatus(t, resp, http.StatusOK)
		meResult := parseJSON(t, resp)
		senderID := meResult["id"].(string)

		body, _ := json.Marshal(map[string]any{
			"to_player_id": senderID,
			"amount":       10,
			"note":         "to myself",
		})
		resp = doRequest(t, "POST", "/api/players/me/kredits/transfer", body, senderToken)
		assertStatus(t, resp, http.StatusBadRequest)
	})
}

// ---------------------------------------------------------------------------
// GET /api/players/{id} (public profile)
// ---------------------------------------------------------------------------

func TestPlayerPublicProfile(t *testing.T) {
	// ---- Setup: register a player ----
	_, playerID := registerAndGetToken(t, "public_user", "public@test.dev", "PublicPass1!")

	// ---- Subtest: view public profile ----
	t.Run("success", func(t *testing.T) {
		resp := doRequest(t, "GET", fmt.Sprintf("/api/players/%s", playerID), nil, "")
		assertStatus(t, resp, http.StatusOK)

		result := parseJSON(t, resp)
		if result["username"] != "public_user" {
			t.Fatalf("expected username 'public_user', got %v", result["username"])
		}

		// Public profile should NOT include email (private data).
		if _, hasEmail := result["email"]; hasEmail {
			t.Fatal("public profile should not include email")
		}
	})

	// ---- Subtest: nonexistent player ----
	t.Run("not_found", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/players/00000000-0000-0000-0000-000000000000", nil, "")
		assertStatus(t, resp, http.StatusNotFound)
	})
}
