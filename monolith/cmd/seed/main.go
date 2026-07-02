// Package main — Database seed script
//
// This program populates the database with realistic test data so you can
// explore the application without manually entering everything.
//
// Run with: go run ./cmd/seed  (from the monolith/ directory)
// Or:        task seed
//
// SEED CREDENTIALS:
//
//	Admin:      username=admin,          email=admin@ark8de.dev,        password=Admin1234!
//	Moderator:  username=moderator,      email=moderator@ark8de.dev,    password=Mod1234!
//
//	Team Alpha (over gear budget — gear pool will show red):
//	  Owner:  username=alpha_captain,  email=alpha.captain@ark8de.dev,  password=Alpha1234!  (tank)
//	  Member: username=alpha_healer,   email=alpha.healer@ark8de.dev,   password=Alpha1234!  (healer)
//	  Member: username=alpha_dps,      email=alpha.dps@ark8de.dev,      password=Alpha1234!  (dps)
//	  Member: username=alpha_support,  email=alpha.support@ark8de.dev,  password=Alpha1234!  (support)
//
//	Team Beta (under gear budget — gear pool shows green):
//	  Owner:  username=beta_captain,   email=beta.captain@ark8de.dev,   password=Beta1234!   (dps)
//	  Member: username=beta_tank,      email=beta.tank@ark8de.dev,      password=Beta1234!   (tank)
//	  Member: username=beta_healer,    email=beta.healer@ark8de.dev,    password=Beta1234!   (healer)
//	  Member: username=beta_support,   email=beta.support@ark8de.dev,   password=Beta1234!   (support)
//
//	Lone wolf (no team):
//	  username=lone_wolf, email=lone.wolf@ark8de.dev, password=Wolf1234!
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
	if err := seedSkills(ctx, pool); err != nil {
		slog.Error("failed to seed skills", "error", err)
		os.Exit(1)
	}

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
	alphaHealer  string
	alphaDPS     string
	alphaSupport string
	betaCaptain  string
	betaTank     string
	betaHealer   string
	betaSupport  string
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
	ids.alphaHealer, err = createPlayer(ctx, pool, "alpha_healer", "alpha.healer@ark8de.dev", "Alpha1234!", "player")
	if err != nil {
		return nil, err
	}
	ids.alphaDPS, err = createPlayer(ctx, pool, "alpha_dps", "alpha.dps@ark8de.dev", "Alpha1234!", "player")
	if err != nil {
		return nil, err
	}
	ids.alphaSupport, err = createPlayer(ctx, pool, "alpha_support", "alpha.support@ark8de.dev", "Alpha1234!", "player")
	if err != nil {
		return nil, err
	}

	// Team Beta — will be seeded with gear UNDER budget (pool is positive)
	ids.betaCaptain, err = createPlayer(ctx, pool, "beta_captain", "beta.captain@ark8de.dev", "Beta1234!", "player")
	if err != nil {
		return nil, err
	}
	ids.betaTank, err = createPlayer(ctx, pool, "beta_tank", "beta.tank@ark8de.dev", "Beta1234!", "player")
	if err != nil {
		return nil, err
	}
	ids.betaHealer, err = createPlayer(ctx, pool, "beta_healer", "beta.healer@ark8de.dev", "Beta1234!", "player")
	if err != nil {
		return nil, err
	}
	ids.betaSupport, err = createPlayer(ctx, pool, "beta_support", "beta.support@ark8de.dev", "Beta1234!", "player")
	if err != nil {
		return nil, err
	}

	// No-team player
	ids.loneWolf, err = createPlayer(ctx, pool, "lone_wolf", "lone.wolf@ark8de.dev", "Wolf1234!", "player")
	if err != nil {
		return nil, err
	}

	// Set class roles on all players.
	classMap := map[string]string{
		ids.alphaCaptain: "tank",
		ids.alphaHealer:  "healer",
		ids.alphaDPS:     "dps",
		ids.alphaSupport: "support",
		ids.betaCaptain:  "dps",
		ids.betaTank:     "tank",
		ids.betaHealer:   "healer",
		ids.betaSupport:  "support",
		ids.loneWolf:     "dps",
	}
	for playerID, class := range classMap {
		if _, err := pool.Exec(ctx,
			`UPDATE players SET class_role = $1 WHERE id = $2::uuid`,
			class, playerID,
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
		[]string{ids.alphaHealer, ids.alphaDPS, ids.alphaSupport})
	if err != nil {
		return fmt.Errorf("seedTeams alpha: %w", err)
	}

	_, err = createTeam(ctx, pool, "Team Beta", "BETA", ids.betaCaptain,
		[]string{ids.betaTank, ids.betaHealer, ids.betaSupport})
	if err != nil {
		return fmt.Errorf("seedTeams beta: %w", err)
	}

	return nil
}

// seedSkills inserts 4 skills per class (16 total). Safe to re-run (skips existing).
func seedSkills(ctx context.Context, pool *pgxpool.Pool) error {
	// Each entry: name, description, class_role, cost, effect_description, effect_type, hp_bonus, armor_bonus
	skills := []struct {
		name, desc, class, effectDesc, effectType string
		cost, hp, armor                           int
	}{
		// Tank skills
		{"Iron Wall", "Brace for impact, hardening your defences", "tank", "+15 armor points", "passive", 3, 0, 15},
		{"Battle Hardened", "Years of combat have thickened your hide", "tank", "+30 HP", "passive", 5, 30, 0},
		{"Shield Mastery", "Master your shield to absorb devastating blows", "tank", "+20 HP and +20 armor", "active", 7, 20, 20},
		{"Fortress", "You become an immovable wall in battle", "tank", "+50 HP and +25 armor", "passive", 10, 50, 25},

		// DPS skills
		{"Quick Reflexes", "Dodge incoming blows with lightning speed", "dps", "+20 HP", "passive", 3, 20, 0},
		{"Battle Fury", "Channel rage into devastating strikes", "dps", "+30 HP and +5 armor", "active", 5, 30, 5},
		{"Deadly Focus", "Laser precision in the heat of combat", "dps", "+15 armor points", "passive", 7, 0, 15},
		{"Glass Cannon", "Maximum offence, minimum defence", "dps", "+60 HP", "passive", 10, 60, 0},

		// Healer skills
		{"First Aid", "Patch wounds mid-combat", "healer", "+25 HP", "passive", 3, 25, 0},
		{"Combat Medic", "Keep fighting while keeping allies alive", "healer", "+35 HP", "active", 5, 35, 0},
		{"Barrier Shield", "Conjure a protective barrier", "healer", "+15 HP and +20 armor", "active", 7, 15, 20},
		{"Divine Blessing", "Divine protection surrounds you", "healer", "+60 HP and +15 armor", "passive", 10, 60, 15},

		// Support skills
		{"Battle Cry", "Rally your team with a mighty roar", "support", "+20 HP and +5 armor", "passive", 3, 20, 5},
		{"Tactical Awareness", "Anticipate enemy movements before they happen", "support", "+25 HP and +10 armor", "passive", 5, 25, 10},
		{"Coordination", "Synchronise attacks with perfect timing", "support", "+30 HP and +15 armor", "active", 7, 30, 15},
		{"Master Tactician", "Command the battlefield from within it", "support", "+50 HP and +25 armor", "passive", 10, 50, 25},
	}

	for _, s := range skills {
		// Skip if already exists.
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM skills WHERE name = $1 AND class_role = $2)`,
			s.name, s.class,
		).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}

		if _, err := pool.Exec(ctx, `
			INSERT INTO skills (name, description, class_role, cost_skill_points,
			                    effect_description, effect_type, hp_bonus, armor_bonus)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			s.name, s.desc, s.class, s.cost, s.effectDesc, s.effectType, s.hp, s.armor,
		); err != nil {
			return fmt.Errorf("insert skill %s: %w", s.name, err)
		}
	}

	slog.Info("skills seeded", "count", 16)
	return nil
}

// seedSkillAllocations gives each player some allocated skills.
func seedSkillAllocations(ctx context.Context, pool *pgxpool.Pool, ids *playerIDs) error {
	// Map: playerID → class → skill names to allocate
	// Budget = 20 skill points per player.
	allocations := map[string]struct {
		class  string
		skills []string // allocate these by name
	}{
		ids.alphaCaptain: {"tank", []string{"Iron Wall", "Battle Hardened"}},       // 3+5=8 pts
		ids.alphaHealer:  {"healer", []string{"First Aid", "Combat Medic"}},        // 3+5=8 pts
		ids.alphaDPS:     {"dps", []string{"Quick Reflexes", "Glass Cannon"}},      // 3+10=13 pts
		ids.alphaSupport: {"support", []string{"Battle Cry", "Coordination"}},      // 3+7=10 pts
		ids.betaCaptain:  {"dps", []string{"Battle Fury", "Deadly Focus"}},         // 5+7=12 pts
		ids.betaTank:     {"tank", []string{"Iron Wall", "Shield Mastery"}},        // 3+7=10 pts
		ids.betaHealer:   {"healer", []string{"First Aid", "Barrier Shield"}},      // 3+7=10 pts
		ids.betaSupport:  {"support", []string{"Battle Cry", "Tactical Awareness"}}, // 3+5=8 pts
	}

	for playerID, alloc := range allocations {
		// Check if already has allocations.
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

		for _, skillName := range alloc.skills {
			var skillID string
			if err := pool.QueryRow(ctx,
				`SELECT id::text FROM skills WHERE name = $1 AND class_role = $2`,
				skillName, alloc.class,
			).Scan(&skillID); err != nil {
				return fmt.Errorf("find skill %s for class %s: %w", skillName, alloc.class, err)
			}
			if _, err := pool.Exec(ctx,
				`INSERT INTO player_skill_allocations (player_id, skill_id) VALUES ($1::uuid, $2::uuid)`,
				playerID, skillID,
			); err != nil {
				return fmt.Errorf("allocate skill %s to player: %w", skillName, err)
			}
		}
	}

	slog.Info("skill allocations seeded")
	return nil
}

// seedGearSelections assigns gear to players.
// Team Alpha deliberately goes OVER budget (pool total=12, used=15) → red warning.
// Team Beta stays UNDER budget (pool total=12, used=9) → healthy.
func seedGearSelections(ctx context.Context, pool *pgxpool.Pool, ids *playerIDs) error {
	// Map: playerID → gear names to equip
	// Gear costs: sword=2, bow_and_arrow=3, spear=4, shield=5
	gearMap := map[string][]string{
		// Team Alpha total: 5 + 4 + 4 + 2 = 15 → OVER budget (12)
		ids.alphaCaptain: {"shield"},                    // 5 pts
		ids.alphaHealer:  {"spear"},                     // 4 pts
		ids.alphaDPS:     {"spear"},                     // 4 pts
		ids.alphaSupport: {"sword"},                     // 2 pts

		// Team Beta total: 2 + 3 + 2 + 2 = 9 → UNDER budget (12)
		ids.betaCaptain:  {"sword"},                     // 2 pts
		ids.betaTank:     {"bow_and_arrow"},              // 3 pts
		ids.betaHealer:   {"sword"},                     // 2 pts
		ids.betaSupport:  {"sword"},                     // 2 pts
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
