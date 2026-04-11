// Package team manages team creation, membership, join requests, lock-in,
// and Arkade point assignment.
//
// This package owns: teams, team_members, join_requests, arkade_point_logs.
// It also writes to leaderboard_entries when team state changes.
package team

import "time"

// Team represents one competing team in the Ark8de.
type Team struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Tag            string    `json:"tag"`      // short display tag e.g. [ARK]
	OwnerID        string    `json:"owner_id"`
	LogoURL        *string   `json:"logo_url"`
	ArkadePoints   int       `json:"arkade_points"`
	GearPointsTotal int      `json:"gear_points_total"` // shared pool budget (default 12)
	IsLockedIn     bool      `json:"is_locked_in"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TeamMember is a player who belongs to a team.
type TeamMember struct {
	PlayerID        string    `json:"player_id"`
	Username        string    `json:"username"`
	ClassRole       *string   `json:"class_role"`
	ProfilePhotoURL *string   `json:"profile_photo_url"`
	JoinedAt        time.Time `json:"joined_at"`
}

// JoinRequest is a player's request to join a team, waiting for owner approval.
type JoinRequest struct {
	ID          string     `json:"id"`
	TeamID      string     `json:"team_id"`
	PlayerID    string     `json:"player_id"`
	Username    string     `json:"username"` // joined from players table for readability
	Status      string     `json:"status"`   // "pending", "accepted", "rejected"
	RequestedAt time.Time  `json:"requested_at"`
	ResolvedAt  *time.Time `json:"resolved_at"`
}

// ArkadePointLog records every change to a team's Arkade point total.
// The log is append-only — entries are never deleted.
type ArkadePointLog struct {
	ID        string    `json:"id"`
	TeamID    string    `json:"team_id"`
	ChangedBy string    `json:"changed_by"` // moderator's player ID
	Delta     int       `json:"delta"`       // positive = awarded, negative = deducted
	Reason    *string   `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// TeamDetailResponse is the full payload for GET /teams/:id.
// It includes the roster and live gear pool status.
type TeamDetailResponse struct {
	*Team
	Members         []TeamMember `json:"members"`
	GearPointsUsed  int          `json:"gear_points_used"`  // sum of all members' gear costs
	GearPoolNegative bool        `json:"gear_pool_negative"` // true = team is over budget
	OwnerUsername   string       `json:"owner_username"`
}

// ---------------------------------------------------------------------------
// Request types
// ---------------------------------------------------------------------------

// CreateTeamRequest is the body for POST /teams.
type CreateTeamRequest struct {
	Name string `json:"name"`
	Tag  string `json:"tag"`
}

// UpdateTeamRequest is the body for PUT /teams/:id.
type UpdateTeamRequest struct {
	Name string `json:"name"`
	Tag  string `json:"tag"`
}

// ResolveJoinRequestBody is the body for PUT /teams/:id/join-requests/:rid.
type ResolveJoinRequestBody struct {
	Action string `json:"action"` // "accept" or "reject"
}

// AddArkadePointsRequest is the body for PUT /teams/:id/points.
type AddArkadePointsRequest struct {
	Delta  int    `json:"delta"`  // positive to add, negative to deduct
	Reason string `json:"reason"`
}
