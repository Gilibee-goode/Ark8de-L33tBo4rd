// Package team — Database Repository layer
//
// All SQL queries for teams, team_members, join_requests, arkade_point_logs,
// and leaderboard_entries (which this package keeps in sync).
package team

import (
	// --- Standard library ---
	"context"
	"fmt"

	// --- Third-party ---
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile-time check: *TeamRepository must satisfy the Repository interface.
// See auth/repository.go for a detailed explanation of this pattern.
var _ Repository = (*TeamRepository)(nil)

// TeamRepository handles all DB access for the team package.
type TeamRepository struct {
	db *pgxpool.Pool
}

// NewTeamRepository constructs a TeamRepository.
func NewTeamRepository(db *pgxpool.Pool) *TeamRepository {
	return &TeamRepository{db: db}
}

// ---------------------------------------------------------------------------
// Team CRUD
// ---------------------------------------------------------------------------

// CreateTeam inserts a new team and its first member (the owner) in one transaction.
// It also upgrades the owner's role to "team_owner" and inserts a leaderboard entry.
func (r *TeamRepository) CreateTeam(ctx context.Context, name, tag, ownerID string) (*Team, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("TeamRepository.CreateTeam: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// Insert the team row and return all columns.
	var t Team
	err = tx.QueryRow(ctx, `
		INSERT INTO teams (name, tag, owner_id)
		VALUES ($1, $2, $3::uuid)
		RETURNING id::text, name, tag, owner_id::text, logo_url,
		          arkade_points, gear_points_total, is_locked_in, created_at, updated_at`,
		name, tag, ownerID,
	).Scan(&t.ID, &t.Name, &t.Tag, &t.OwnerID, &t.LogoURL,
		&t.ArkadePoints, &t.GearPointsTotal, &t.IsLockedIn, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("TeamRepository.CreateTeam: insert team: %w", err)
	}

	// Add the owner as the first member.
	_, err = tx.Exec(ctx,
		`INSERT INTO team_members (team_id, player_id) VALUES ($1::uuid, $2::uuid)`,
		t.ID, ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("TeamRepository.CreateTeam: add owner member: %w", err)
	}

	// Promote the owner's role to "team_owner".
	_, err = tx.Exec(ctx,
		`UPDATE players SET role = 'team_owner', updated_at = NOW() WHERE id = $1::uuid`,
		ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("TeamRepository.CreateTeam: promote owner role: %w", err)
	}

	// Create the leaderboard entry for this team (starts at 0 points, 1 member).
	_, err = tx.Exec(ctx, `
		INSERT INTO leaderboard_entries (team_id, team_name, team_tag, arkade_points, member_count)
		VALUES ($1::uuid, $2, $3, 0, 1)`,
		t.ID, t.Name, t.Tag,
	)
	if err != nil {
		return nil, fmt.Errorf("TeamRepository.CreateTeam: leaderboard entry: %w", err)
	}

	return &t, tx.Commit(ctx)
}

// GetTeam fetches a team by ID. Returns pgx.ErrNoRows if not found.
func (r *TeamRepository) GetTeam(ctx context.Context, teamID string) (*Team, error) {
	var t Team
	err := r.db.QueryRow(ctx, `
		SELECT id::text, name, tag, owner_id::text, logo_url,
		       arkade_points, gear_points_total, is_locked_in, created_at, updated_at
		FROM   teams WHERE id = $1::uuid`,
		teamID,
	).Scan(&t.ID, &t.Name, &t.Tag, &t.OwnerID, &t.LogoURL,
		&t.ArkadePoints, &t.GearPointsTotal, &t.IsLockedIn, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("TeamRepository.GetTeam: %w", err)
	}
	return &t, nil
}

// ListTeams returns all teams, sorted by Arkade points descending.
func (r *TeamRepository) ListTeams(ctx context.Context) ([]*Team, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, name, tag, owner_id::text, logo_url,
		       arkade_points, gear_points_total, is_locked_in, created_at, updated_at
		FROM   teams
		ORDER BY arkade_points DESC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("TeamRepository.ListTeams: %w", err)
	}
	defer rows.Close()

	var teams []*Team
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.Name, &t.Tag, &t.OwnerID, &t.LogoURL,
			&t.ArkadePoints, &t.GearPointsTotal, &t.IsLockedIn, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("TeamRepository.ListTeams: scan: %w", err)
		}
		teams = append(teams, &t)
	}
	return teams, nil
}

// UpdateTeam changes a team's name and tag.
func (r *TeamRepository) UpdateTeam(ctx context.Context, teamID, name, tag string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE teams SET name = $1, tag = $2, updated_at = NOW() WHERE id = $3::uuid`,
		name, tag, teamID,
	)
	if err != nil {
		return fmt.Errorf("TeamRepository.UpdateTeam: %w", err)
	}
	return nil
}

// DeleteTeam removes the team and reverts the owner's role to "player".
// ON DELETE CASCADE handles team_members, join_requests, and leaderboard_entries.
func (r *TeamRepository) DeleteTeam(ctx context.Context, teamID, ownerID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("TeamRepository.DeleteTeam: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// Revert owner role only if they are currently team_owner (not mod/admin).
	_, err = tx.Exec(ctx, `
		UPDATE players SET role = 'player', updated_at = NOW()
		WHERE id = $1::uuid AND role = 'team_owner'`,
		ownerID,
	)
	if err != nil {
		return fmt.Errorf("TeamRepository.DeleteTeam: revert role: %w", err)
	}

	// Deleting the team cascades to team_members, join_requests, leaderboard_entries.
	_, err = tx.Exec(ctx, `DELETE FROM teams WHERE id = $1::uuid`, teamID)
	if err != nil {
		return fmt.Errorf("TeamRepository.DeleteTeam: delete: %w", err)
	}

	return tx.Commit(ctx)
}

// ToggleLock flips the is_locked_in flag on a team.
func (r *TeamRepository) ToggleLock(ctx context.Context, teamID string) (bool, error) {
	var newState bool
	err := r.db.QueryRow(ctx, `
		UPDATE teams
		SET    is_locked_in = NOT is_locked_in, updated_at = NOW()
		WHERE  id = $1::uuid
		RETURNING is_locked_in`,
		teamID,
	).Scan(&newState)
	if err != nil {
		return false, fmt.Errorf("TeamRepository.ToggleLock: %w", err)
	}
	return newState, nil
}

// ---------------------------------------------------------------------------
// Team members
// ---------------------------------------------------------------------------

// GetMembers returns all members of a team with their profile info.
func (r *TeamRepository) GetMembers(ctx context.Context, teamID string) ([]TeamMember, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id::text, p.username, p.class_role, p.profile_photo_url, tm.joined_at
		FROM   team_members tm
		JOIN   players p ON p.id = tm.player_id
		WHERE  tm.team_id = $1::uuid
		ORDER BY tm.joined_at ASC`,
		teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("TeamRepository.GetMembers: %w", err)
	}
	defer rows.Close()

	var members []TeamMember
	for rows.Next() {
		var m TeamMember
		if err := rows.Scan(&m.PlayerID, &m.Username, &m.ClassRole, &m.ProfilePhotoURL, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("TeamRepository.GetMembers: scan: %w", err)
		}
		members = append(members, m)
	}
	return members, nil
}

// GetTeamGearUsed sums the gear point costs for all members of a team.
func (r *TeamRepository) GetTeamGearUsed(ctx context.Context, teamID string) (int, error) {
	var total int
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(gt.gear_point_cost), 0)
		FROM   team_members tm
		LEFT JOIN player_gear pg ON pg.player_id = tm.player_id
		LEFT JOIN gear_types gt ON gt.id = pg.gear_type_id
		WHERE  tm.team_id = $1::uuid`,
		teamID,
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("TeamRepository.GetTeamGearUsed: %w", err)
	}
	return total, nil
}

// RemoveMember removes a player from a team and decrements the leaderboard member count.
func (r *TeamRepository) RemoveMember(ctx context.Context, teamID, playerID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("TeamRepository.RemoveMember: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`DELETE FROM team_members WHERE team_id = $1::uuid AND player_id = $2::uuid`,
		teamID, playerID,
	)
	if err != nil {
		return fmt.Errorf("TeamRepository.RemoveMember: delete: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE leaderboard_entries
		SET    member_count = GREATEST(member_count - 1, 0), last_updated_at = NOW()
		WHERE  team_id = $1::uuid`,
		teamID,
	)
	if err != nil {
		return fmt.Errorf("TeamRepository.RemoveMember: leaderboard: %w", err)
	}

	return tx.Commit(ctx)
}

// IsMember returns true if playerID is currently a member of any team.
func (r *TeamRepository) IsMember(ctx context.Context, playerID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM team_members WHERE player_id = $1::uuid)`,
		playerID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("TeamRepository.IsMember: %w", err)
	}
	return exists, nil
}

// GetMemberCount returns the number of members in a team.
func (r *TeamRepository) GetMemberCount(ctx context.Context, teamID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM team_members WHERE team_id = $1::uuid`,
		teamID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("TeamRepository.GetMemberCount: %w", err)
	}
	return count, nil
}

// GetOwnerUsername fetches only the owner's username for a team detail response.
func (r *TeamRepository) GetOwnerUsername(ctx context.Context, ownerID string) (string, error) {
	var username string
	err := r.db.QueryRow(ctx,
		`SELECT username FROM players WHERE id = $1::uuid`,
		ownerID,
	).Scan(&username)
	if err != nil {
		return "", fmt.Errorf("TeamRepository.GetOwnerUsername: %w", err)
	}
	return username, nil
}

// ---------------------------------------------------------------------------
// Join requests
// ---------------------------------------------------------------------------

// CreateJoinRequest inserts a new pending join request.
func (r *TeamRepository) CreateJoinRequest(ctx context.Context, teamID, playerID string) (*JoinRequest, error) {
	var req JoinRequest
	err := r.db.QueryRow(ctx, `
		INSERT INTO join_requests (team_id, player_id)
		VALUES ($1::uuid, $2::uuid)
		RETURNING id::text, team_id::text, player_id::text, status, requested_at, resolved_at`,
		teamID, playerID,
	).Scan(&req.ID, &req.TeamID, &req.PlayerID, &req.Status, &req.RequestedAt, &req.ResolvedAt)
	if err != nil {
		return nil, fmt.Errorf("TeamRepository.CreateJoinRequest: %w", err)
	}
	req.Username = "" // populated by service if needed
	return &req, nil
}

// GetPendingJoinRequests returns all pending join requests for a team, with the requester's username.
func (r *TeamRepository) GetPendingJoinRequests(ctx context.Context, teamID string) ([]JoinRequest, error) {
	rows, err := r.db.Query(ctx, `
		SELECT jr.id::text, jr.team_id::text, jr.player_id::text,
		       p.username, jr.status, jr.requested_at, jr.resolved_at
		FROM   join_requests jr
		JOIN   players p ON p.id = jr.player_id
		WHERE  jr.team_id = $1::uuid AND jr.status = 'pending'
		ORDER BY jr.requested_at ASC`,
		teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("TeamRepository.GetPendingJoinRequests: %w", err)
	}
	defer rows.Close()

	var requests []JoinRequest
	for rows.Next() {
		var req JoinRequest
		if err := rows.Scan(&req.ID, &req.TeamID, &req.PlayerID,
			&req.Username, &req.Status, &req.RequestedAt, &req.ResolvedAt); err != nil {
			return nil, fmt.Errorf("TeamRepository.GetPendingJoinRequests: scan: %w", err)
		}
		requests = append(requests, req)
	}
	return requests, nil
}

// GetJoinRequest fetches a specific join request by ID.
func (r *TeamRepository) GetJoinRequest(ctx context.Context, requestID string) (*JoinRequest, error) {
	var req JoinRequest
	err := r.db.QueryRow(ctx, `
		SELECT jr.id::text, jr.team_id::text, jr.player_id::text,
		       p.username, jr.status, jr.requested_at, jr.resolved_at
		FROM   join_requests jr
		JOIN   players p ON p.id = jr.player_id
		WHERE  jr.id = $1::uuid`,
		requestID,
	).Scan(&req.ID, &req.TeamID, &req.PlayerID, &req.Username,
		&req.Status, &req.RequestedAt, &req.ResolvedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("TeamRepository.GetJoinRequest: %w", err)
	}
	return &req, nil
}

// AcceptJoinRequest accepts a join request in one transaction:
// updates the request status, adds the player to the team, rejects their other pending
// requests, and increments the leaderboard member count.
func (r *TeamRepository) AcceptJoinRequest(ctx context.Context, requestID, teamID, playerID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("TeamRepository.AcceptJoinRequest: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	now := "NOW()"
	_ = now // used inline below

	// Mark this request as accepted.
	_, err = tx.Exec(ctx, `
		UPDATE join_requests SET status = 'accepted', resolved_at = NOW()
		WHERE  id = $1::uuid`,
		requestID,
	)
	if err != nil {
		return fmt.Errorf("TeamRepository.AcceptJoinRequest: update request: %w", err)
	}

	// Add the player to the team.
	_, err = tx.Exec(ctx,
		`INSERT INTO team_members (team_id, player_id) VALUES ($1::uuid, $2::uuid)`,
		teamID, playerID,
	)
	if err != nil {
		return fmt.Errorf("TeamRepository.AcceptJoinRequest: insert member: %w", err)
	}

	// Reject all OTHER pending requests by this player to other teams.
	_, err = tx.Exec(ctx, `
		UPDATE join_requests
		SET    status = 'rejected', resolved_at = NOW()
		WHERE  player_id = $1::uuid AND status = 'pending' AND id != $2::uuid`,
		playerID, requestID,
	)
	if err != nil {
		return fmt.Errorf("TeamRepository.AcceptJoinRequest: reject others: %w", err)
	}

	// Increment leaderboard member count.
	_, err = tx.Exec(ctx, `
		UPDATE leaderboard_entries
		SET    member_count = member_count + 1, last_updated_at = NOW()
		WHERE  team_id = $1::uuid`,
		teamID,
	)
	if err != nil {
		return fmt.Errorf("TeamRepository.AcceptJoinRequest: leaderboard: %w", err)
	}

	return tx.Commit(ctx)
}

// RejectJoinRequest marks a join request as rejected.
func (r *TeamRepository) RejectJoinRequest(ctx context.Context, requestID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE join_requests SET status = 'rejected', resolved_at = NOW()
		WHERE  id = $1::uuid`,
		requestID,
	)
	if err != nil {
		return fmt.Errorf("TeamRepository.RejectJoinRequest: %w", err)
	}
	return nil
}

// HasPendingRequest returns true if the player already has a pending request for this team.
func (r *TeamRepository) HasPendingRequest(ctx context.Context, teamID, playerID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM join_requests
			WHERE team_id = $1::uuid AND player_id = $2::uuid AND status = 'pending'
		)`,
		teamID, playerID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("TeamRepository.HasPendingRequest: %w", err)
	}
	return exists, nil
}

// ---------------------------------------------------------------------------
// Arkade points
// ---------------------------------------------------------------------------

// AddArkadePoints adds delta points to a team (delta may be negative for deductions).
// Atomically updates the team, inserts the audit log, and updates the leaderboard.
// After updating, it recalculates all team ranks.
func (r *TeamRepository) AddArkadePoints(ctx context.Context, teamID, changedBy string, delta int, reason string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("TeamRepository.AddArkadePoints: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// Update the team's total.
	_, err = tx.Exec(ctx, `
		UPDATE teams SET arkade_points = arkade_points + $1, updated_at = NOW()
		WHERE  id = $2::uuid`,
		delta, teamID,
	)
	if err != nil {
		return fmt.Errorf("TeamRepository.AddArkadePoints: update team: %w", err)
	}

	// Insert the audit log entry.
	_, err = tx.Exec(ctx, `
		INSERT INTO arkade_point_logs (team_id, changed_by, delta, reason)
		VALUES ($1::uuid, $2::uuid, $3, NULLIF($4, ''))`,
		teamID, changedBy, delta, reason,
	)
	if err != nil {
		return fmt.Errorf("TeamRepository.AddArkadePoints: insert log: %w", err)
	}

	// Sync the leaderboard entry with the new point total.
	_, err = tx.Exec(ctx, `
		UPDATE leaderboard_entries le
		SET    arkade_points = t.arkade_points, last_updated_at = NOW()
		FROM   teams t
		WHERE  le.team_id = t.id AND t.id = $1::uuid`,
		teamID,
	)
	if err != nil {
		return fmt.Errorf("TeamRepository.AddArkadePoints: sync leaderboard: %w", err)
	}

	// Recalculate ranks for ALL teams using a window function.
	// RANK() assigns the same rank to tied teams; teams below a tie skip a rank number.
	// This single UPDATE recalculates all teams in one query.
	_, err = tx.Exec(ctx, `
		UPDATE leaderboard_entries le
		SET    rank = sub.new_rank
		FROM (
			SELECT team_id,
			       RANK() OVER (ORDER BY arkade_points DESC) AS new_rank
			FROM   leaderboard_entries
		) sub
		WHERE le.team_id = sub.team_id`)
	if err != nil {
		return fmt.Errorf("TeamRepository.AddArkadePoints: recalculate ranks: %w", err)
	}

	return tx.Commit(ctx)
}

// GetArkadePointHistory returns all point change events for a team, newest first.
func (r *TeamRepository) GetArkadePointHistory(ctx context.Context, teamID string) ([]ArkadePointLog, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, team_id::text, changed_by::text, delta, reason, created_at
		FROM   arkade_point_logs
		WHERE  team_id = $1::uuid
		ORDER BY created_at DESC`,
		teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("TeamRepository.GetArkadePointHistory: %w", err)
	}
	defer rows.Close()

	var logs []ArkadePointLog
	for rows.Next() {
		var l ArkadePointLog
		if err := rows.Scan(&l.ID, &l.TeamID, &l.ChangedBy, &l.Delta, &l.Reason, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("TeamRepository.GetArkadePointHistory: scan: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, nil
}
