// Package leaderboard provides a read-optimised view of team rankings.
//
// The leaderboard_entries table is a denormalised cache — it stores pre-computed
// team data (name, tag, points, member count, rank) so the leaderboard page
// never needs to JOIN teams + team_members + aggregate at query time.
//
// This package only READS — it never writes. Writes happen in the team package
// whenever points or membership change.
package leaderboard

import "time"

// LeaderboardEntry is one row in the ranked team list.
type LeaderboardEntry struct {
	TeamID        string    `json:"team_id"`
	TeamName      string    `json:"team_name"`
	TeamTag       string    `json:"team_tag"`
	TeamLogoURL   *string   `json:"team_logo_url"`
	ArkadePoints  int       `json:"arkade_points"`
	Rank          *int      `json:"rank"` // nil until the first points change
	MemberCount   int       `json:"member_count"`
	LastUpdatedAt time.Time `json:"last_updated_at"`
}
