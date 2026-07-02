// Package player — Database Repository layer
//
// This file contains all SQL queries for the player package.
// It touches: players, skills, player_skill_allocations,
//             gear_types, player_gear, kredit_transactions.
//
// No business logic lives here — only data access.
package player

import (
	// --- Standard library ---
	"context"
	"fmt"

	// --- Third-party ---
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile-time check: *PlayerRepository must satisfy the Repository interface.
// See auth/repository.go for a detailed explanation of this pattern.
var _ Repository = (*PlayerRepository)(nil)

// PlayerRepository handles all DB access for the player package.
type PlayerRepository struct {
	db *pgxpool.Pool
}

// NewPlayerRepository constructs a PlayerRepository.
func NewPlayerRepository(db *pgxpool.Pool) *PlayerRepository {
	return &PlayerRepository{db: db}
}

// ---------------------------------------------------------------------------
// Class role
// ---------------------------------------------------------------------------

// UpdateClassRole sets the player's class and clears their skill allocations,
// since skills are class-specific. Changing class invalidates all prior allocations.
// Returns the new class_role string.
func (r *PlayerRepository) UpdateClassRole(ctx context.Context, playerID, classRole string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("PlayerRepository.UpdateClassRole: begin tx: %w", err)
	}
	// defer Rollback is safe to call even after Commit — it becomes a no-op.
	defer tx.Rollback(ctx)

	// Clear all existing skill allocations for this player.
	// We must do this before setting the new class because skills are validated
	// against the player's class — old skills may be illegal for the new class.
	_, err = tx.Exec(ctx,
		`DELETE FROM player_skill_allocations WHERE player_id = $1::uuid`,
		playerID,
	)
	if err != nil {
		return fmt.Errorf("PlayerRepository.UpdateClassRole: clear skills: %w", err)
	}

	// Set the new class role.
	_, err = tx.Exec(ctx,
		`UPDATE players SET class_role = $1, updated_at = NOW() WHERE id = $2::uuid`,
		classRole, playerID,
	)
	if err != nil {
		return fmt.Errorf("PlayerRepository.UpdateClassRole: update class: %w", err)
	}

	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// Skills
// ---------------------------------------------------------------------------

// GetSkillsByClass returns all available skills for a given class role.
// This is the list players choose from when allocating skills.
func (r *PlayerRepository) GetSkillsByClass(ctx context.Context, classRole string) ([]Skill, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, class_role, branch, tier, name, description,
		       hp_bonus, team_gear_bonus
		FROM   skills
		WHERE  ($1 = '' OR class_role = $1)
		ORDER BY class_role, tier, branch`,
		classRole,
	)
	if err != nil {
		return nil, fmt.Errorf("PlayerRepository.GetSkillsByClass: %w", err)
	}
	defer rows.Close()

	skills, err := scanSkills(rows)
	if err != nil {
		return nil, fmt.Errorf("PlayerRepository.GetSkillsByClass: %w", err)
	}
	return skills, nil
}

// scanSkills reads skill rows into a slice — shared by all skill queries,
// which must SELECT the same columns in the same order.
func scanSkills(rows pgx.Rows) ([]Skill, error) {
	var skills []Skill
	for rows.Next() {
		var s Skill
		if err := rows.Scan(&s.ID, &s.ClassRole, &s.Branch, &s.Tier,
			&s.Name, &s.Description, &s.HPBonus, &s.TeamGearBonus); err != nil {
			return nil, fmt.Errorf("scan skill: %w", err)
		}
		skills = append(skills, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return skills, nil
}

// GetPlayerSkills returns all skills currently allocated by a player.
func (r *PlayerRepository) GetPlayerSkills(ctx context.Context, playerID string) ([]Skill, error) {
	rows, err := r.db.Query(ctx, `
		SELECT s.id::text, s.class_role, s.branch, s.tier, s.name, s.description,
		       s.hp_bonus, s.team_gear_bonus
		FROM   player_skill_allocations psa
		JOIN   skills s ON s.id = psa.skill_id
		WHERE  psa.player_id = $1::uuid
		ORDER BY s.tier, s.branch`,
		playerID,
	)
	if err != nil {
		return nil, fmt.Errorf("PlayerRepository.GetPlayerSkills: %w", err)
	}
	defer rows.Close()

	skills, err := scanSkills(rows)
	if err != nil {
		return nil, fmt.Errorf("PlayerRepository.GetPlayerSkills: %w", err)
	}
	return skills, nil
}

// SetPlayerSkills replaces a player's skill allocations with a new set.
// It runs inside a transaction: delete all existing → insert new ones.
// If skillIDs is empty, all allocations are cleared.
func (r *PlayerRepository) SetPlayerSkills(ctx context.Context, playerID string, skillIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("PlayerRepository.SetPlayerSkills: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Delete all current allocations for this player.
	_, err = tx.Exec(ctx,
		`DELETE FROM player_skill_allocations WHERE player_id = $1::uuid`,
		playerID,
	)
	if err != nil {
		return fmt.Errorf("PlayerRepository.SetPlayerSkills: delete: %w", err)
	}

	// Insert each new skill allocation.
	for _, skillID := range skillIDs {
		_, err = tx.Exec(ctx, `
			INSERT INTO player_skill_allocations (player_id, skill_id)
			VALUES ($1::uuid, $2::uuid)`,
			playerID, skillID,
		)
		if err != nil {
			return fmt.Errorf("PlayerRepository.SetPlayerSkills: insert skill %s: %w", skillID, err)
		}
	}

	return tx.Commit(ctx)
}

// GetSkillsByIDs fetches multiple skills by their IDs in one query.
// Used during skill validation to verify all requested skill IDs exist.
func (r *PlayerRepository) GetSkillsByIDs(ctx context.Context, ids []string) ([]Skill, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// pgx converts []string to a PostgreSQL text array for ANY().
	rows, err := r.db.Query(ctx, `
		SELECT id::text, class_role, branch, tier, name, description,
		       hp_bonus, team_gear_bonus
		FROM   skills
		WHERE  id::text = ANY($1)`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("PlayerRepository.GetSkillsByIDs: %w", err)
	}
	defer rows.Close()

	skills, err := scanSkills(rows)
	if err != nil {
		return nil, fmt.Errorf("PlayerRepository.GetSkillsByIDs: %w", err)
	}
	return skills, nil
}

// ---------------------------------------------------------------------------
// Gear
// ---------------------------------------------------------------------------

// GetAllGearTypes returns every weapon in the catalog.
// Used to let players browse available equipment.
func (r *PlayerRepository) GetAllGearTypes(ctx context.Context) ([]GearType, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id::text, name, gear_point_cost, restricted_to FROM gear_types ORDER BY gear_point_cost ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("PlayerRepository.GetAllGearTypes: %w", err)
	}
	defer rows.Close()

	var gear []GearType
	for rows.Next() {
		var g GearType
		if err := rows.Scan(&g.ID, &g.Name, &g.GearPointCost, &g.RestrictedTo); err != nil {
			return nil, fmt.Errorf("PlayerRepository.GetAllGearTypes: scan: %w", err)
		}
		gear = append(gear, g)
	}
	return gear, nil
}

// GetPlayerGear returns the gear types currently selected by a player.
func (r *PlayerRepository) GetPlayerGear(ctx context.Context, playerID string) ([]GearType, error) {
	rows, err := r.db.Query(ctx, `
		SELECT gt.id::text, gt.name, gt.gear_point_cost, gt.restricted_to
		FROM   player_gear pg
		JOIN   gear_types gt ON gt.id = pg.gear_type_id
		WHERE  pg.player_id = $1::uuid
		ORDER BY gt.gear_point_cost ASC`,
		playerID,
	)
	if err != nil {
		return nil, fmt.Errorf("PlayerRepository.GetPlayerGear: %w", err)
	}
	defer rows.Close()

	var gear []GearType
	for rows.Next() {
		var g GearType
		if err := rows.Scan(&g.ID, &g.Name, &g.GearPointCost, &g.RestrictedTo); err != nil {
			return nil, fmt.Errorf("PlayerRepository.GetPlayerGear: scan: %w", err)
		}
		gear = append(gear, g)
	}
	return gear, nil
}

// SetPlayerGear replaces a player's gear selection. Runs in a transaction.
func (r *PlayerRepository) SetPlayerGear(ctx context.Context, playerID string, gearTypeIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("PlayerRepository.SetPlayerGear: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`DELETE FROM player_gear WHERE player_id = $1::uuid`,
		playerID,
	)
	if err != nil {
		return fmt.Errorf("PlayerRepository.SetPlayerGear: delete: %w", err)
	}

	for _, gearID := range gearTypeIDs {
		_, err = tx.Exec(ctx, `
			INSERT INTO player_gear (player_id, gear_type_id)
			VALUES ($1::uuid, $2::uuid)`,
			playerID, gearID,
		)
		if err != nil {
			return fmt.Errorf("PlayerRepository.SetPlayerGear: insert gear %s: %w", gearID, err)
		}
	}

	return tx.Commit(ctx)
}

// GetGearTypesByIDs fetches multiple gear types by their IDs.
// Used during gear selection validation.
func (r *PlayerRepository) GetGearTypesByIDs(ctx context.Context, ids []string) ([]GearType, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT id::text, name, gear_point_cost, restricted_to FROM gear_types WHERE id::text = ANY($1)`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("PlayerRepository.GetGearTypesByIDs: %w", err)
	}
	defer rows.Close()

	var gear []GearType
	for rows.Next() {
		var g GearType
		if err := rows.Scan(&g.ID, &g.Name, &g.GearPointCost, &g.RestrictedTo); err != nil {
			return nil, fmt.Errorf("PlayerRepository.GetGearTypesByIDs: scan: %w", err)
		}
		gear = append(gear, g)
	}
	return gear, nil
}

// GetTeamGearPointsUsed returns the total gear points consumed by all members
// of the team that contains playerID. Returns (teamGearTotal, teamGearUsed, error).
// If the player is not on a team, both ints are 0.
func (r *PlayerRepository) GetTeamGearPointsUsed(ctx context.Context, playerID string) (teamTotal, teamUsed int, err error) {
	// Find which team this player belongs to and sum all gear costs for all members.
	// The CTE (WITH clause) first finds the player's team, then sums gear.
	// CTE = Common Table Expression — a named sub-query that makes complex SQL readable.
	row := r.db.QueryRow(ctx, `
		WITH my_team AS (
			SELECT tm.team_id, t.gear_points_total
			FROM   team_members tm
			JOIN   teams t ON t.id = tm.team_id
			WHERE  tm.player_id = $1::uuid
			LIMIT  1
		)
		SELECT
			-- Team pool = base total + skill bonuses held by members
			-- (the hacker's "Grid is Good" grants team_gear_bonus = 4).
			COALESCE(mt.gear_points_total, 0) + COALESCE((
				SELECT COALESCE(SUM(s.team_gear_bonus), 0)
				FROM   team_members tm2
				JOIN   player_skill_allocations psa ON psa.player_id = tm2.player_id
				JOIN   skills s ON s.id = psa.skill_id
				WHERE  tm2.team_id = mt.team_id
			), 0),
			COALESCE(SUM(gt.gear_point_cost), 0)
		FROM   my_team mt
		LEFT JOIN team_members all_tm ON all_tm.team_id = mt.team_id
		LEFT JOIN player_gear pg ON pg.player_id = all_tm.player_id
		LEFT JOIN gear_types gt ON gt.id = pg.gear_type_id
		GROUP BY mt.team_id, mt.gear_points_total`,
		playerID,
	)

	if err := row.Scan(&teamTotal, &teamUsed); err != nil {
		if err == pgx.ErrNoRows {
			// Player is not on any team — that's fine, return zeros.
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("PlayerRepository.GetTeamGearPointsUsed: %w", err)
	}
	return teamTotal, teamUsed, nil
}

// ---------------------------------------------------------------------------
// Player profile (public)
// ---------------------------------------------------------------------------

// GetPublicProfile fetches the publicly visible fields of any player.
func (r *PlayerRepository) GetPublicProfile(ctx context.Context, playerID string) (*PublicPlayerResponse, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, username, role, profile_photo_url, class_role, level
		FROM   players
		WHERE  id = $1::uuid`,
		playerID,
	)
	var p PublicPlayerResponse
	if err := row.Scan(&p.ID, &p.Username, &p.Role, &p.ProfilePhotoURL, &p.ClassRole, &p.Level); err != nil {
		if err == pgx.ErrNoRows {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("PlayerRepository.GetPublicProfile: %w", err)
	}
	return &p, nil
}

// GetPlayerCore fetches the minimal player fields needed for stats and skill
// validation: class, level, and the archetype's passive HP bonus (0 if the
// player has not picked a class yet).
func (r *PlayerRepository) GetPlayerCore(ctx context.Context, playerID string) (classRole *string, level int, passiveHPBonus int, err error) {
	row := r.db.QueryRow(ctx, `
		SELECT p.class_role, p.level, COALESCE(a.hp_bonus, 0)
		FROM   players p
		LEFT JOIN archetypes a ON a.class_role = p.class_role
		WHERE  p.id = $1::uuid`,
		playerID,
	)
	if scanErr := row.Scan(&classRole, &level, &passiveHPBonus); scanErr != nil {
		return nil, 0, 0, fmt.Errorf("PlayerRepository.GetPlayerCore: %w", scanErr)
	}
	return classRole, level, passiveHPBonus, nil
}

// SetPlayerLevel updates a player's level and prunes any skill allocations
// whose tier is now above the new level (relevant when leveling down).
func (r *PlayerRepository) SetPlayerLevel(ctx context.Context, playerID string, level int) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("PlayerRepository.SetPlayerLevel: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`UPDATE players SET level = $1, updated_at = NOW() WHERE id = $2::uuid`,
		level, playerID,
	)
	if err != nil {
		return fmt.Errorf("PlayerRepository.SetPlayerLevel: update: %w", err)
	}

	// Remove allocations the player no longer qualifies for.
	_, err = tx.Exec(ctx, `
		DELETE FROM player_skill_allocations psa
		USING  skills s
		WHERE  psa.skill_id = s.id
		  AND  psa.player_id = $1::uuid
		  AND  s.tier > $2`,
		playerID, level,
	)
	if err != nil {
		return fmt.Errorf("PlayerRepository.SetPlayerLevel: prune skills: %w", err)
	}

	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// Kredits
// ---------------------------------------------------------------------------

// GetKreditBalance returns just the current Kredit balance for a player.
func (r *PlayerRepository) GetKreditBalance(ctx context.Context, playerID string) (int, error) {
	var balance int
	err := r.db.QueryRow(ctx,
		`SELECT kredits FROM players WHERE id = $1::uuid`,
		playerID,
	).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("PlayerRepository.GetKreditBalance: %w", err)
	}
	return balance, nil
}

// GetKreditTransactions returns all Kredit transactions involving a player
// (as sender or receiver), most recent first.
func (r *PlayerRepository) GetKreditTransactions(ctx context.Context, playerID string) ([]KreditTransaction, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text,
		       from_player_id::text,
		       to_player_id::text,
		       amount,
		       note,
		       created_by::text,
		       created_at
		FROM   kredit_transactions
		WHERE  to_player_id = $1::uuid OR from_player_id = $1::uuid
		ORDER BY created_at DESC
		LIMIT  100`,
		playerID,
	)
	if err != nil {
		return nil, fmt.Errorf("PlayerRepository.GetKreditTransactions: %w", err)
	}
	defer rows.Close()

	var txns []KreditTransaction
	for rows.Next() {
		var t KreditTransaction
		// from_player_id is nullable — scan into *string to handle NULL correctly.
		var fromID *string
		if err := rows.Scan(&t.ID, &fromID, &t.ToPlayerID, &t.Amount,
			&t.Note, &t.CreatedBy, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("PlayerRepository.GetKreditTransactions: scan: %w", err)
		}
		t.FromPlayerID = fromID
		txns = append(txns, t)
	}
	return txns, nil
}

// GrantKredits adds Kredits to a player (moderator grant: no sender).
// Atomically inserts the transaction log and increments the player's balance.
func (r *PlayerRepository) GrantKredits(ctx context.Context, toPlayerID, createdBy string, amount int, note string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("PlayerRepository.GrantKredits: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Insert the audit log entry. from_player_id = NULL indicates a mod grant.
	_, err = tx.Exec(ctx, `
		INSERT INTO kredit_transactions (from_player_id, to_player_id, amount, note, created_by)
		VALUES (NULL, $1::uuid, $2, NULLIF($3, ''), $4::uuid)`,
		toPlayerID, amount, note, createdBy,
	)
	if err != nil {
		return fmt.Errorf("PlayerRepository.GrantKredits: insert log: %w", err)
	}

	// Increment the recipient's balance.
	_, err = tx.Exec(ctx,
		`UPDATE players SET kredits = kredits + $1, updated_at = NOW() WHERE id = $2::uuid`,
		amount, toPlayerID,
	)
	if err != nil {
		return fmt.Errorf("PlayerRepository.GrantKredits: update balance: %w", err)
	}

	return tx.Commit(ctx)
}

// TransferKredits moves Kredits from one player to another.
// Fails if the sender has insufficient balance (enforced by DB CHECK).
func (r *PlayerRepository) TransferKredits(ctx context.Context, fromPlayerID, toPlayerID, createdBy string, amount int, note string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("PlayerRepository.TransferKredits: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Deduct from sender. The DB CHECK (kredits >= 0) will reject this if they
	// can't afford it, which gives us a pgconn.PgError with code "23514" (check_violation).
	_, err = tx.Exec(ctx,
		`UPDATE players SET kredits = kredits - $1, updated_at = NOW() WHERE id = $2::uuid`,
		amount, fromPlayerID,
	)
	if err != nil {
		return fmt.Errorf("PlayerRepository.TransferKredits: deduct: %w", err)
	}

	// Credit the receiver.
	_, err = tx.Exec(ctx,
		`UPDATE players SET kredits = kredits + $1, updated_at = NOW() WHERE id = $2::uuid`,
		amount, toPlayerID,
	)
	if err != nil {
		return fmt.Errorf("PlayerRepository.TransferKredits: credit: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO kredit_transactions (from_player_id, to_player_id, amount, note, created_by)
		VALUES ($1::uuid, $2::uuid, $3, NULLIF($4, ''), $5::uuid)`,
		fromPlayerID, toPlayerID, amount, note, createdBy,
	)
	if err != nil {
		return fmt.Errorf("PlayerRepository.TransferKredits: insert log: %w", err)
	}

	return tx.Commit(ctx)
}

// PlayerExists returns true if a player with the given ID exists.
func (r *PlayerRepository) PlayerExists(ctx context.Context, playerID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM players WHERE id = $1::uuid)`,
		playerID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("PlayerRepository.PlayerExists: %w", err)
	}
	return exists, nil
}
