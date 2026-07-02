// Package leaderboard — Business Logic / Service layer
package leaderboard

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrTeamNotFound is returned when no leaderboard entry exists for the given team ID.
var ErrTeamNotFound = errors.New("team not found on leaderboard")

// LeaderboardService provides read access to team rankings.
type LeaderboardService struct {
	repo *LeaderboardRepository
}

// NewLeaderboardService constructs a LeaderboardService.
func NewLeaderboardService(repo *LeaderboardRepository) *LeaderboardService {
	return &LeaderboardService{repo: repo}
}

// GetLeaderboard returns all teams in ranked order.
func (s *LeaderboardService) GetLeaderboard(ctx context.Context) ([]LeaderboardEntry, error) {
	entries, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("LeaderboardService.GetLeaderboard: %w", err)
	}
	if entries == nil {
		entries = []LeaderboardEntry{}
	}
	return entries, nil
}

// GetTeamCard returns the leaderboard entry for a single team.
func (s *LeaderboardService) GetTeamCard(ctx context.Context, teamID string) (*LeaderboardEntry, error) {
	entry, err := s.repo.GetByTeamID(ctx, teamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTeamNotFound
		}
		return nil, fmt.Errorf("LeaderboardService.GetTeamCard: %w", err)
	}
	return entry, nil
}
