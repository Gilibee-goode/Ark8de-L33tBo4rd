// Package tests — Player endpoint integration tests
//
// This file tests the player-related API endpoints:
//   PUT  /api/players/me/class            — set archetype (smartass/ninja/psycho/hacker/merkava/kommando)
//   GET  /api/players/me/stats            — view computed combat stats (HP, level, gear points)
//   GET  /api/players/me/skills           — view allocated skills
//   PUT  /api/players/me/skills           — allocate skills (one per tier, blue or red)
//   PUT  /api/players/{id}/level          — set player level (moderator only)
//   GET  /api/players/me/gear             — view selected weapons
//   PUT  /api/players/me/gear             — select weapons (class restrictions enforced)
//   GET  /api/players/me/kredits          — view Kredit balance and history
//   POST /api/players/me/kredits/transfer — transfer Kredits to another player
//   GET  /api/players/{id}                — public player profile
//   GET  /api/skills                      — list skill trees (public)
//
// Skills and the weapon catalog are reference data seeded by migration 000014,
// so they are always present once migrations run — no per-test seeding needed.
package tests

import (
	// --- Standard library ---
	"context"       // context.Background for DB lookups
	"encoding/json" // json.Marshal builds JSON request bodies
	"fmt"           // fmt.Sprintf builds URL paths with player IDs
	"net/http"      // HTTP status constants
	"testing"       // Go's test framework
)

// skillID looks up one skill in the migration-seeded tree by class/branch/tier.
func skillID(t *testing.T, class, branch string, tier int) string {
	t.Helper()
	var id string
	err := testPool.QueryRow(context.Background(),
		`SELECT id::text FROM skills WHERE class_role = $1 AND branch = $2 AND tier = $3`,
		class, branch, tier,
	).Scan(&id)
	if err != nil {
		t.Fatalf("failed to find skill %s/%s/%d: %v", class, branch, tier, err)
	}
	return id
}

// gearID looks up one weapon in the migration-seeded catalog by name.
func gearID(t *testing.T, name string) string {
	t.Helper()
	var id string
	err := testPool.QueryRow(context.Background(),
		`SELECT id::text FROM gear_types WHERE name = $1`, name,
	).Scan(&id)
	if err != nil {
		t.Fatalf("failed to find gear type %q: %v", name, err)
	}
	return id
}

// setLevelDirect raises a player's level via SQL — used to set up test states
// without going through the moderator endpoint (which has its own test).
func setLevelDirect(t *testing.T, playerID string, level int) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		`UPDATE players SET level = $1 WHERE id = $2::uuid`, level, playerID,
	); err != nil {
		t.Fatalf("failed to set level: %v", err)
	}
}

// ---------------------------------------------------------------------------
// PUT /api/players/me/class
// ---------------------------------------------------------------------------

func TestPlayerSetClass(t *testing.T) {
	// ---- Setup: register a player ----
	token, _ := registerAndGetToken(t, "class_user", "class@test.dev", "ClassPass1!")

	// ---- Subtest: set class to merkava ----
	t.Run("set_merkava", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"class_role": "merkava"})
		resp := doRequest(t, "PUT", "/api/players/me/class", body, token)
		assertStatus(t, resp, http.StatusOK)
	})

	// ---- Subtest: switch archetype (clears any allocated skills) ----
	t.Run("switch_to_ninja", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"class_role": "ninja"})
		resp := doRequest(t, "PUT", "/api/players/me/class", body, token)
		assertStatus(t, resp, http.StatusOK)
	})

	// ---- Subtest: invalid class ----
	t.Run("invalid_class", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"class_role": "wizard"})
		resp := doRequest(t, "PUT", "/api/players/me/class", body, token)
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// ---- Subtest: old class names are no longer valid ----
	t.Run("old_class_rejected", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"class_role": "tank"})
		resp := doRequest(t, "PUT", "/api/players/me/class", body, token)
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// ---- Subtest: no auth ----
	t.Run("no_auth", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"class_role": "merkava"})
		resp := doRequest(t, "PUT", "/api/players/me/class", body, "")
		assertStatus(t, resp, http.StatusUnauthorized)
	})
}

// ---------------------------------------------------------------------------
// GET /api/players/me/stats
// ---------------------------------------------------------------------------

func TestPlayerGetStats(t *testing.T) {
	// ---- Setup: register a player and set the merkava archetype ----
	token, _ := registerAndGetToken(t, "stats_user", "stats@test.dev", "StatsPass1!")

	body, _ := json.Marshal(map[string]string{"class_role": "merkava"})
	resp := doRequest(t, "PUT", "/api/players/me/class", body, token)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// ---- Subtest: stats with a class set ----
	t.Run("with_class", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/players/me/stats", nil, token)
		assertStatus(t, resp, http.StatusOK)

		result := parseJSON(t, resp)

		if result["class_role"] != "merkava" {
			t.Fatalf("expected class_role 'merkava', got %v", result["class_role"])
		}

		// Merkava: BaseHP (3) + passive (+1) = 4, no skills yet.
		hp := result["health_points"].(float64)
		if hp != 4 {
			t.Fatalf("expected 4 HP (base 3 + merkava passive 1), got %v", hp)
		}

		// New players start at level 1.
		level := result["level"].(float64)
		if level != 1 {
			t.Fatalf("expected level 1, got %v", level)
		}
	})
}

// ---------------------------------------------------------------------------
// GET/PUT /api/players/me/skills
// ---------------------------------------------------------------------------

func TestPlayerSkills(t *testing.T) {
	// ---- Setup: register a merkava player (level 1 by default) ----
	token, playerID := registerAndGetToken(t, "skills_user", "skills@test.dev", "SkillsPass1!")

	body, _ := json.Marshal(map[string]string{"class_role": "merkava"})
	resp := doRequest(t, "PUT", "/api/players/me/class", body, token)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	fridge := skillID(t, "merkava", "blue", 1)          // tier 1 blue (+1 HP)
	nailed := skillID(t, "merkava", "red", 1)           // tier 1 red
	windbreaker := skillID(t, "merkava", "blue", 2)     // tier 2 blue
	ninjaSkill := skillID(t, "ninja", "blue", 1)        // wrong class

	// ---- Subtest: allocate a tier-1 skill at level 1 ----
	t.Run("allocate_tier1", func(t *testing.T) {
		body, _ := json.Marshal(map[string][]string{"skill_ids": {fridge}})
		resp := doRequest(t, "PUT", "/api/players/me/skills", body, token)
		assertStatus(t, resp, http.StatusOK)
	})

	// ---- Subtest: tier 2 requires level 2 ----
	t.Run("tier_above_level", func(t *testing.T) {
		body, _ := json.Marshal(map[string][]string{"skill_ids": {fridge, windbreaker}})
		resp := doRequest(t, "PUT", "/api/players/me/skills", body, token)
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// ---- Subtest: blue AND red of the same tier is illegal ----
	t.Run("one_skill_per_tier", func(t *testing.T) {
		setLevelDirect(t, playerID, 2)
		body, _ := json.Marshal(map[string][]string{"skill_ids": {fridge, nailed}})
		resp := doRequest(t, "PUT", "/api/players/me/skills", body, token)
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// ---- Subtest: at level 2, tier 1 + tier 2 is legal ----
	t.Run("allocate_two_tiers", func(t *testing.T) {
		body, _ := json.Marshal(map[string][]string{"skill_ids": {fridge, windbreaker}})
		resp := doRequest(t, "PUT", "/api/players/me/skills", body, token)
		assertStatus(t, resp, http.StatusOK)
	})

	// ---- Subtest: get allocated skills ----
	t.Run("get_skills", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/players/me/skills", nil, token)
		assertStatus(t, resp, http.StatusOK)

		result := parseJSON(t, resp)
		skills := result["allocated_skills"].([]any)
		if len(skills) != 2 {
			t.Fatalf("expected 2 allocated skills, got %d", len(skills))
		}
		if result["level"].(float64) != 2 {
			t.Fatalf("expected level 2, got %v", result["level"])
		}
	})

	// ---- Subtest: stats include the Fridge HP bonus ----
	t.Run("stats_with_fridge", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/players/me/stats", nil, token)
		assertStatus(t, resp, http.StatusOK)

		result := parseJSON(t, resp)
		// BaseHP 3 + merkava passive 1 + Fridge 1 = 5.
		if hp := result["health_points"].(float64); hp != 5 {
			t.Fatalf("expected 5 HP, got %v", hp)
		}
	})

	// ---- Subtest: wrong class skill ----
	t.Run("wrong_class_skill", func(t *testing.T) {
		body, _ := json.Marshal(map[string][]string{"skill_ids": {ninjaSkill}})
		resp := doRequest(t, "PUT", "/api/players/me/skills", body, token)
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// ---- Subtest: nonexistent skill ID ----
	t.Run("invalid_skill_id", func(t *testing.T) {
		body, _ := json.Marshal(map[string][]string{
			"skill_ids": {"00000000-0000-0000-0000-000000000000"},
		})
		resp := doRequest(t, "PUT", "/api/players/me/skills", body, token)
		assertStatus(t, resp, http.StatusBadRequest)
	})
}

// ---------------------------------------------------------------------------
// PUT /api/players/{id}/level (moderator only)
// ---------------------------------------------------------------------------

func TestSetPlayerLevel(t *testing.T) {
	// ---- Setup: a player and a moderator ----
	playerToken, playerID := registerAndGetToken(t, "lvl_user", "lvl@test.dev", "LvlPass1!")
	registerAndGetToken(t, "lvl_mod", "lvl_mod@test.dev", "LvlModPass1!")
	if _, err := testPool.Exec(context.Background(),
		"UPDATE players SET role = 'moderator' WHERE username = 'lvl_mod'",
	); err != nil {
		t.Fatalf("failed to promote moderator: %v", err)
	}
	modToken := loginPlayer(t, "lvl_mod@test.dev", "LvlModPass1!")

	// Give the player a class and a tier-1 + tier-2 allocation at level 2,
	// so we can verify level-down pruning.
	body, _ := json.Marshal(map[string]string{"class_role": "kommando"})
	resp := doRequest(t, "PUT", "/api/players/me/class", body, playerToken)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// ---- Subtest: moderator raises the level ----
	t.Run("moderator_sets_level", func(t *testing.T) {
		body, _ := json.Marshal(map[string]int{"level": 2})
		resp := doRequest(t, "PUT", fmt.Sprintf("/api/players/%s/level", playerID), body, modToken)
		assertStatus(t, resp, http.StatusOK)
	})

	// ---- Subtest: leveling down prunes out-of-reach skills ----
	t.Run("level_down_prunes_skills", func(t *testing.T) {
		// Allocate tier 1 + tier 2 at level 2.
		alloc, _ := json.Marshal(map[string][]string{"skill_ids": {
			skillID(t, "kommando", "blue", 1),
			skillID(t, "kommando", "red", 2),
		}})
		resp := doRequest(t, "PUT", "/api/players/me/skills", alloc, playerToken)
		assertStatus(t, resp, http.StatusOK)
		resp.Body.Close()

		// Moderator drops the player to level 1 — the tier-2 skill must go.
		body, _ := json.Marshal(map[string]int{"level": 1})
		resp = doRequest(t, "PUT", fmt.Sprintf("/api/players/%s/level", playerID), body, modToken)
		assertStatus(t, resp, http.StatusOK)
		resp.Body.Close()

		resp = doRequest(t, "GET", "/api/players/me/skills", nil, playerToken)
		assertStatus(t, resp, http.StatusOK)
		result := parseJSON(t, resp)
		skills := result["allocated_skills"].([]any)
		if len(skills) != 1 {
			t.Fatalf("expected 1 skill after level-down prune, got %d", len(skills))
		}
	})

	// ---- Subtest: invalid level ----
	t.Run("invalid_level", func(t *testing.T) {
		body, _ := json.Marshal(map[string]int{"level": 7})
		resp := doRequest(t, "PUT", fmt.Sprintf("/api/players/%s/level", playerID), body, modToken)
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// ---- Subtest: regular players cannot set levels ----
	t.Run("player_forbidden", func(t *testing.T) {
		body, _ := json.Marshal(map[string]int{"level": 3})
		resp := doRequest(t, "PUT", fmt.Sprintf("/api/players/%s/level", playerID), body, playerToken)
		assertStatus(t, resp, http.StatusForbidden)
	})
}

// ---------------------------------------------------------------------------
// GET /api/skills (public)
// ---------------------------------------------------------------------------

func TestListSkills(t *testing.T) {
	// ---- Subtest: no filter returns the full tree (36 skills from migration) ----
	t.Run("no_filter_returns_all", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/skills", nil, "")
		assertStatus(t, resp, http.StatusOK)

		skills := parseJSONArray(t, resp)
		if len(skills) != 36 {
			t.Fatalf("expected 36 skills (6 archetypes x 2 branches x 3 tiers), got %d", len(skills))
		}
	})

	// ---- Subtest: filter by archetype ----
	t.Run("filter_by_class", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/skills?class_role=psycho", nil, "")
		assertStatus(t, resp, http.StatusOK)

		skills := parseJSONArray(t, resp)
		if len(skills) != 6 {
			t.Fatalf("expected 6 psycho skills, got %d", len(skills))
		}
		for _, s := range skills {
			skill := s.(map[string]any)
			if skill["class_role"] != "psycho" {
				t.Fatalf("expected class_role 'psycho', got %v", skill["class_role"])
			}
		}
	})
}

// ---------------------------------------------------------------------------
// GET/PUT /api/players/me/gear
// ---------------------------------------------------------------------------

func TestPlayerGear(t *testing.T) {
	// ---- Setup: register a merkava player (may equip shields) ----
	token, _ := registerAndGetToken(t, "gear_user", "gear@test.dev", "GearPass1!")
	body, _ := json.Marshal(map[string]string{"class_role": "merkava"})
	resp := doRequest(t, "PUT", "/api/players/me/class", body, token)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	daggerID := gearID(t, "dagger")
	shieldID := gearID(t, "shield")

	// ---- Subtest: select gear ----
	t.Run("select_gear", func(t *testing.T) {
		body, _ := json.Marshal(map[string][]string{
			"gear_type_ids": {daggerID, shieldID},
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

		// Gear points used by me: dagger(1) + shield(4) = 5.
		myPoints := result["gear_points_used_by_me"].(float64)
		if myPoints != 5 {
			t.Fatalf("expected 5 gear points used, got %v", myPoints)
		}
	})

	// ---- Subtest: class restrictions enforced ----
	// A ninja may not equip a shield.
	t.Run("class_restricted_gear", func(t *testing.T) {
		ninjaToken, _ := registerAndGetToken(t, "gear_ninja", "gear_ninja@test.dev", "GearPass1!")
		body, _ := json.Marshal(map[string]string{"class_role": "ninja"})
		resp := doRequest(t, "PUT", "/api/players/me/class", body, ninjaToken)
		assertStatus(t, resp, http.StatusOK)
		resp.Body.Close()

		body, _ = json.Marshal(map[string][]string{"gear_type_ids": {shieldID}})
		resp = doRequest(t, "PUT", "/api/players/me/gear", body, ninjaToken)
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// ---- Subtest: power balls are psycho/hacker only ----
	t.Run("power_balls_restricted", func(t *testing.T) {
		powerBallsID := gearID(t, "power_balls_x4")

		// The merkava player may not take power balls...
		body, _ := json.Marshal(map[string][]string{"gear_type_ids": {powerBallsID}})
		resp := doRequest(t, "PUT", "/api/players/me/gear", body, token)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()

		// ...but a psycho may.
		psychoToken, _ := registerAndGetToken(t, "gear_psycho", "gear_psycho@test.dev", "GearPass1!")
		body2, _ := json.Marshal(map[string]string{"class_role": "psycho"})
		resp = doRequest(t, "PUT", "/api/players/me/class", body2, psychoToken)
		assertStatus(t, resp, http.StatusOK)
		resp.Body.Close()

		body3, _ := json.Marshal(map[string][]string{"gear_type_ids": {powerBallsID}})
		resp = doRequest(t, "PUT", "/api/players/me/gear", body3, psychoToken)
		assertStatus(t, resp, http.StatusOK)
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

		// Public profiles include the player's level (starts at 1).
		if result["level"].(float64) != 1 {
			t.Fatalf("expected level 1, got %v", result["level"])
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
