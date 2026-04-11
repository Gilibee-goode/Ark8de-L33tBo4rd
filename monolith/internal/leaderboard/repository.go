// Package leaderboard — Database Repository layer
//
// Read-only queries against leaderboard_entries.
// Writes to this table are handled by the team package.
package leaderboard

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LeaderboardRepository handles read queries for the leaderboard.
type LeaderboardRepository struct {
	db *pgxpool.Pool
}

// NewLeaderboardRepository constructs a LeaderboardRepository.
func NewLeaderboardRepository(db *pgxpool.Pool) *LeaderboardRepository {
	return &LeaderboardRepository{db: db}
}

// GetAll returns all leaderboard entries ranked by Arkade points descending.
func (r *LeaderboardRepository) GetAll(ctx context.Context) ([]LeaderboardEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT team_id::text, team_name, team_tag, team_logo_url,
		       arkade_points, rank, member_count, last_updated_at
		FROM   leaderboard_entries
		ORDER BY arkade_points DESC, team_name ASC`)
	if err != nil {
		return nil, fmt.Errorf("LeaderboardRepository.GetAll: %w", err)
	}
	defer rows.Close()

	var entries []LeaderboardEntry
	for rows.Next() {
		var e LeaderboardEntry
		if err := rows.Scan(&e.TeamID, &e.TeamName, &e.TeamTag, &e.TeamLogoURL,
			&e.ArkadePoints, &e.Rank, &e.MemberCount, &e.LastUpdatedAt); err != nil {
			return nil, fmt.Errorf("LeaderboardRepository.GetAll: scan: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// GetByTeamID returns the leaderboard entry for a single team.
func (r *LeaderboardRepository) GetByTeamID(ctx context.Context, teamID string) (*LeaderboardEntry, error) {
	var e LeaderboardEntry
	err := r.db.QueryRow(ctx, `
		SELECT team_id::text, team_name, team_tag, team_logo_url,
		       arkade_points, rank, member_count, last_updated_at
		FROM   leaderboard_entries
		WHERE  team_id = $1::uuid`,
		teamID,
	).Scan(&e.TeamID, &e.TeamName, &e.TeamTag, &e.TeamLogoURL,
		&e.ArkadePoints, &e.Rank, &e.MemberCount, &e.LastUpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("LeaderboardRepository.GetByTeamID: %w", err)
	}
	return &e, nil
}
