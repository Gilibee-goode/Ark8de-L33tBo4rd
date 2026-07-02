// Package main — Database seed script
//
// This program populates the database with realistic test data so you can
// explore the application without manually entering everything.
//
// Run with: go run ./cmd/seed  (from the monolith/ directory)
// Or:        task seed
//
// Skills and the weapon catalog are seeded by migration 000014 — this script
// only creates accounts, teams, allocations, gear picks, and Arkade points.
//
// SEED CREDENTIALS:
//
//	Admin:      username=admin,          email=admin@ark8de.dev,        password=Admin1234!
//	Moderator:  username=moderator,      email=moderator@ark8de.dev,    password=Mod1234!
//
//	Team Alpha (over gear budget — gear pool will show red):
//	  Owner:  username=alpha_captain,  email=alpha.captain@ark8de.dev,  password=Alpha1234!  (merkava, lvl 3)
//	  Member: username=alpha_ninja,    email=alpha.ninja@ark8de.dev,    password=Alpha1234!  (ninja,   lvl 2)
//	  Member: username=alpha_psycho,   email=alpha.psycho@ark8de.dev,   password=Alpha1234!  (psycho,  lvl 1)
//	  Member: username=alpha_hacker,   email=alpha.hacker@ark8de.dev,   password=Alpha1234!  (hacker,  lvl 3)
//
//	Team Beta (under gear budget — gear pool shows green):
//	  Owner:  username=beta_captain,   email=beta.captain@ark8de.dev,   password=Beta1234!   (kommando, lvl 3)
//	  Member: username=beta_smartass,  email=beta.smartass@ark8de.dev,  password=Beta1234!   (smartass, lvl 2)
//	  Member: username=beta_merkava,   email=beta.merkava@ark8de.dev,   password=Beta1234!   (merkava,  lvl 1)
//	  Member: username=beta_ninja,     email=beta.ninja@ark8de.dev,     password=Beta1234!   (ninja,    lvl 1)
//
//	Lone wolf (no team):
//	  username=lone_wolf, email=lone.wolf@ark8de.dev, password=Wolf1234!  (psycho, lvl 2)
package main

import (
	// --- Standard library ---
	"context"
	"fmt"
	"log/slog"
	"os"

	// --- Third-party ---
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	// Load .env from the monolith directory (where this command is run from).
	if err := godotenv.Load(); err != nil {
		slog.Info("no .env file found, reading from environment")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		slog.Error("DATABASE_URL not set")
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	slog.Info("connected to database, starting seed...")

	// Run each section in order. If any fails, we abort.
	// (Skills + weapon catalog are reference data seeded by migration 000014.)
	playerIDs, err := seedPlayers(ctx, pool)
	if err != nil {
		slog.Error("failed to seed players", "error", err)
		os.Exit(1)
	}

	if err := seedTeams(ctx, pool, playerIDs); err != nil {
		slog.Error("failed to seed teams", "error", err)
		os.Exit(1)
	}

	if err := seedSkillAllocations(ctx, pool, playerIDs); err != nil {
		slog.Error("failed to seed skill allocations", "error", err)
		os.Exit(1)
	}

	if err := seedGearSelections(ctx, pool, playerIDs); err != nil {
		slog.Error("failed to seed gear selections", "error", err)
		os.Exit(1)
	}

	if err := seedArkadePoints(ctx, pool, playerIDs); err != nil {
		slog.Error("failed to seed Arkade points", "error", err)
		os.Exit(1)
	}

	slog.Info("seed complete! all test data inserted.")
	fmt.Println("\n=== SEED CREDENTIALS ===")
	fmt.Println("Admin:         admin / Admin1234!")
	fmt.Println("Moderator:     moderator / Mod1234!")
	fmt.Println("Alpha Captain: alpha_captain / Alpha1234!")
	fmt.Println("Beta Captain:  beta_captain / Beta1234!")
	fmt.Println("Lone Wolf:     lone_wolf / Wolf1234!")
}

// playerIDs holds UUIDs for all seeded players so later steps can reference them.
type playerIDs struct {
	admin        string
	moderator    string
	alphaCaptain string
	alphaNinja   string
	alphaPsycho  string
	alphaHacker  string
	betaCaptain  string
	betaSmartass string
	betaMerkava  string
	betaNinja    string
	loneWolf     string
}

// hash runs bcrypt on the given password and returns the hash string.
// We use DefaultCost (10) which takes ~100ms — fast enough for seed but secure.
func hash(password string) string {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(fmt.Sprintf("bcrypt failed: %v", err))
	}
	return string(h)
}

// createPlayer inserts one player and returns their UUID.
// ON CONFLICT DO NOTHING means the seed is safe to re-run — existing rows are skipped.
func createPlayer(ctx context.Context, pool *pgxpool.Pool, username, email, password, role string) (string, error) {
	// First check if the player already exists.
	var id string
	err := pool.QueryRow(ctx,
		`SELECT id::text FROM players WHERE email = $1`,
		email,
	).Scan(&id)
	if err == nil {
		slog.Info("player already exists, skipping", "username", username)
		return id, nil
	}

	// Insert and return the new UUID.
	err = pool.QueryRow(ctx, `
		INSERT INTO players (username, email, password_hash, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text`,
		username, email, hash(password), role,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("createPlayer %s: %w", username, err)
	}
	slog.Info("created player", "username", username, "role", role)
	return id, nil
}

// seedPlayers creates all test accounts and returns their IDs.
func seedPlayers(ctx context.Context, pool *pgxpool.Pool) (*playerIDs, error) {
	ids := &playerIDs{}
	var err error

	ids.admin, err = createPlayer(ctx, pool, "admin", "admin@ark8de.dev", "Admin1234!", "admin")
	if err != nil {
		return nil, err
	}
	ids.moderator, err = createPlayer(ctx, pool, "moderator", "moderator@ark8de.dev", "Mod1234!", "moderator")
	if err != nil {
		return nil, err
	}

	// Team Alpha — will be seeded with gear OVER budget (pool goes negative = red warning)
	ids.alphaCaptain, err = createPlayer(ctx, pool, "alpha_captain", "alpha.captain@ark8de.dev", "Alpha1234!", "player")
	if err != nil {
		return nil, err
	}
	ids.alphaNinja, err = createPlayer(ctx, pool, "alpha_ninja", "alpha.ninja@ark8de.dev", "Alpha1234!", "player")
	if err != nil {
		return nil, err
	}
	ids.alphaPsycho, err = createPlayer(ctx, pool, "alpha_psycho", "alpha.psycho@ark8de.dev", "Alpha1234!", "player")
	if err != nil {
		return nil, err
	}
	ids.alphaHacker, err = createPlayer(ctx, pool, "alpha_hacker", "alpha.hacker@ark8de.dev", "Alpha1234!", "player")
	if err != nil {
		return nil, err
	}

	// Team Beta — will be seeded with gear UNDER budget (pool is positive)
	ids.betaCaptain, err = createPlayer(ctx, pool, "beta_captain", "beta.captain@ark8de.dev", "Beta1234!", "player")
	if err != nil {
		return nil, err
	}
	ids.betaSmartass, err = createPlayer(ctx, pool, "beta_smartass", "beta.smartass@ark8de.dev", "Beta1234!", "player")
	if err != nil {
		return nil, err
	}
	ids.betaMerkava, err = createPlayer(ctx, pool, "beta_merkava", "beta.merkava@ark8de.dev", "Beta1234!", "player")
	if err != nil {
		return nil, err
	}
	ids.betaNinja, err = createPlayer(ctx, pool, "beta_ninja", "beta.ninja@ark8de.dev", "Beta1234!", "player")
	if err != nil {
		return nil, err
	}

	// No-team player
	ids.loneWolf, err = createPlayer(ctx, pool, "lone_wolf", "lone.wolf@ark8de.dev", "Wolf1234!", "player")
	if err != nil {
		return nil, err
	}

	// Set archetype + level on all players. All 6 archetypes are represented.
	classMap := map[string]struct {
		class string
		level int
	}{
		ids.alphaCaptain: {"merkava", 3},
		ids.alphaNinja:   {"ninja", 2},
		ids.alphaPsycho:  {"psycho", 1},
		ids.alphaHacker:  {"hacker", 3},
		ids.betaCaptain:  {"kommando", 3},
		ids.betaSmartass: {"smartass", 2},
		ids.betaMerkava:  {"merkava", 1},
		ids.betaNinja:    {"ninja", 1},
		ids.loneWolf:     {"psycho", 2},
	}
	for playerID, pc := range classMap {
		if _, err := pool.Exec(ctx,
			`UPDATE players SET class_role = $1, level = $2 WHERE id = $3::uuid`,
			pc.class, pc.level, playerID,
		); err != nil {
			return nil, fmt.Errorf("set class for %s: %w", playerID, err)
		}
	}

	return ids, nil
}

// createTeam inserts a team, its members, sets the owner's role, and creates the leaderboard entry.
func createTeam(ctx context.Context, pool *pgxpool.Pool, name, tag, ownerID string, memberIDs []string) (string, error) {
	// Check if team already exists.
	var teamID string
	err := pool.QueryRow(ctx, `SELECT id::text FROM teams WHERE name = $1`, name).Scan(&teamID)
	if err == nil {
		slog.Info("team already exists, skipping", "name", name)
		return teamID, nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("createTeam begin: %w", err)
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		INSERT INTO teams (name, tag, owner_id)
		VALUES ($1, $2, $3::uuid)
		RETURNING id::text`,
		name, tag, ownerID,
	).Scan(&teamID)
	if err != nil {
		return "", fmt.Errorf("createTeam insert: %w", err)
	}

	// Add all members (including owner).
	allMembers := append([]string{ownerID}, memberIDs...)
	for _, pid := range allMembers {
		if _, err := tx.Exec(ctx,
			`INSERT INTO team_members (team_id, player_id) VALUES ($1::uuid, $2::uuid)`,
			teamID, pid,
		); err != nil {
			return "", fmt.Errorf("createTeam add member: %w", err)
		}
	}

	// Promote owner to team_owner.
	if _, err := tx.Exec(ctx,
		`UPDATE players SET role = 'team_owner' WHERE id = $1::uuid`,
		ownerID,
	); err != nil {
		return "", fmt.Errorf("createTeam promote owner: %w", err)
	}

	// Create leaderboard entry.
	if _, err := tx.Exec(ctx, `
		INSERT INTO leaderboard_entries (team_id, team_name, team_tag, arkade_points, member_count)
		VALUES ($1::uuid, $2, $3, 0, $4)`,
		teamID, name, tag, len(allMembers),
	); err != nil {
		return "", fmt.Errorf("createTeam leaderboard: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}

	slog.Info("created team", "name", name, "members", len(allMembers))
	return teamID, nil
}

// seedTeams creates Team Alpha and Team Beta with their rosters.
func seedTeams(ctx context.Context, pool *pgxpool.Pool, ids *playerIDs) error {
	_, err := createTeam(ctx, pool, "Team Alpha", "ALPHA", ids.alphaCaptain,
		[]string{ids.alphaNinja, ids.alphaPsycho, ids.alphaHacker})
	if err != nil {
		return fmt.Errorf("seedTeams alpha: %w", err)
	}

	_, err = createTeam(ctx, pool, "Team Beta", "BETA", ids.betaCaptain,
		[]string{ids.betaSmartass, ids.betaMerkava, ids.betaNinja})
	if err != nil {
		return fmt.Errorf("seedTeams beta: %w", err)
	}

	return nil
}

// pick identifies one skill-tree node: which branch to take at which tier.
type pick struct {
	branch string // "blue" or "red"
	tier   int
}

// seedSkillAllocations gives each player skills following the real rules:
// one skill per tier (blue OR red), tiers up to the player's level.
func seedSkillAllocations(ctx context.Context, pool *pgxpool.Pool, ids *playerIDs) error {
	allocations := map[string]struct {
		class string
		picks []pick
	}{
		ids.alphaCaptain: {"merkava", []pick{{"blue", 1}, {"blue", 2}, {"red", 3}}},  // Fridge, Windbreaker, Brain Glitch
		ids.alphaNinja:   {"ninja", []pick{{"blue", 1}, {"red", 2}}},                 // The Elusive, The Coward
		ids.alphaPsycho:  {"psycho", []pick{{"blue", 1}}},                            // Basic Psychosis
		ids.alphaHacker:  {"hacker", []pick{{"blue", 1}, {"red", 2}, {"blue", 3}}},   // Cheats, It's a Bug, Grid is Good (+4 team gear)
		ids.betaCaptain:  {"kommando", []pick{{"red", 1}, {"blue", 2}, {"red", 3}}},  // The Pusher, Adrenaline Shot, Friend of the Quartermaster
		ids.betaSmartass: {"smartass", []pick{{"red", 1}, {"blue", 2}}},              // The Coward, Red Bull Shot
		ids.betaMerkava:  {"merkava", []pick{{"red", 1}}},                            // Nailed to the Floor
		ids.loneWolf:     {"psycho", []pick{{"red", 1}, {"blue", 2}}},                // Rechargeable Batteries, Psychic Armor
		// beta_ninja deliberately has no skills yet.
	}

	for playerID, alloc := range allocations {
		// Check if already has allocations (safe re-run).
		var count int
		if err := pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM player_skill_allocations WHERE player_id = $1::uuid`,
			playerID,
		).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}

		for _, p := range alloc.picks {
			var skillID string
			if err := pool.QueryRow(ctx,
				`SELECT id::text FROM skills WHERE class_role = $1 AND branch = $2 AND tier = $3`,
				alloc.class, p.branch, p.tier,
			).Scan(&skillID); err != nil {
				return fmt.Errorf("find %s %s tier %d: %w", alloc.class, p.branch, p.tier, err)
			}
			if _, err := pool.Exec(ctx,
				`INSERT INTO player_skill_allocations (player_id, skill_id) VALUES ($1::uuid, $2::uuid)`,
				playerID, skillID,
			); err != nil {
				return fmt.Errorf("allocate %s %s tier %d: %w", alloc.class, p.branch, p.tier, err)
			}
		}
	}

	slog.Info("skill allocations seeded")
	return nil
}

// seedGearSelections assigns weapons to players, respecting class restrictions.
// Weapon costs: short_weapon=1, dagger=1, long_weapon=2, spear=3, bow=4,
// pistol=4, shield=4, power_balls_x4=1.
// Team Alpha goes OVER budget: 7+4+2+4 = 17 vs pool 12+4 (Grid is Good) = 16 → red.
// Team Beta stays UNDER budget: 2+1+4+1 = 8 vs pool 12 → green.
func seedGearSelections(ctx context.Context, pool *pgxpool.Pool, ids *playerIDs) error {
	gearMap := map[string][]string{
		// Team Alpha
		ids.alphaCaptain: {"shield", "spear"},                  // merkava — 4+3 = 7
		ids.alphaNinja:   {"bow"},                              // ninja   — 4
		ids.alphaPsycho:  {"power_balls_x4", "short_weapon"},   // psycho  — 1+1 = 2
		ids.alphaHacker:  {"pistol"},                           // hacker  — 4

		// Team Beta
		ids.betaCaptain:  {"long_weapon"},  // kommando — 2
		ids.betaSmartass: {"dagger"},       // smartass — 1
		ids.betaMerkava:  {"shield"},       // merkava  — 4
		ids.betaNinja:    {"short_weapon"}, // ninja    — 1
	}

	for playerID, gearNames := range gearMap {
		// Skip if already has gear.
		var count int
		if err := pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM player_gear WHERE player_id = $1::uuid`,
			playerID,
		).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}

		for _, gearName := range gearNames {
			var gearID string
			if err := pool.QueryRow(ctx,
				`SELECT id::text FROM gear_types WHERE name = $1`,
				gearName,
			).Scan(&gearID); err != nil {
				return fmt.Errorf("find gear %s: %w", gearName, err)
			}
			if _, err := pool.Exec(ctx,
				`INSERT INTO player_gear (player_id, gear_type_id) VALUES ($1::uuid, $2::uuid)`,
				playerID, gearID,
			); err != nil {
				return fmt.Errorf("equip gear %s for player: %w", gearName, err)
			}
		}
	}

	slog.Info("gear selections seeded")
	return nil
}

// seedArkadePoints gives the teams some starting points so the leaderboard is interesting.
func seedArkadePoints(ctx context.Context, pool *pgxpool.Pool, ids *playerIDs) error {
	// Check if points already exist for alpha team.
	var logCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM arkade_point_logs`,
	).Scan(&logCount); err != nil {
		return err
	}
	if logCount > 0 {
		slog.Info("Arkade point logs already exist, skipping")
		return nil
	}

	type pointEntry struct {
		teamName string
		delta    int
		reason   string
	}

	events := []pointEntry{
		{"Team Alpha", 25, "Won the first round"},
		{"Team Alpha", 10, "Bonus for tactics"},
		{"Team Beta", 30, "Won the second round"},
		{"Team Beta", -5, "Penalty for rule violation"},
		{"Team Alpha", 15, "Won the third round"},
	}

	for _, e := range events {
		// Get team ID and current points.
		var teamID string
		var currentPoints int
		if err := pool.QueryRow(ctx,
			`SELECT id::text, arkade_points FROM teams WHERE name = $1`,
			e.teamName,
		).Scan(&teamID, &currentPoints); err != nil {
			return fmt.Errorf("find team %s: %w", e.teamName, err)
		}

		newPoints := currentPoints + e.delta

		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx,
			`UPDATE teams SET arkade_points = $1 WHERE id = $2::uuid`,
			newPoints, teamID,
		); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("update team points: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO arkade_point_logs (team_id, changed_by, delta, reason)
			VALUES ($1::uuid, $2::uuid, $3, $4)`,
			teamID, ids.moderator, e.delta, e.reason,
		); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("insert log: %w", err)
		}

		if _, err := tx.Exec(ctx,
			`UPDATE leaderboard_entries SET arkade_points = $1, last_updated_at = NOW() WHERE team_id = $2::uuid`,
			newPoints, teamID,
		); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("update leaderboard: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}

	// Recalculate ranks.
	if _, err := pool.Exec(ctx, `
		UPDATE leaderboard_entries le
		SET    rank = sub.new_rank
		FROM (
			SELECT team_id, RANK() OVER (ORDER BY arkade_points DESC) AS new_rank
			FROM   leaderboard_entries
		) sub
		WHERE le.team_id = sub.team_id`); err != nil {
		return fmt.Errorf("recalculate ranks: %w", err)
	}

	slog.Info("Arkade points seeded")
	return nil
}
