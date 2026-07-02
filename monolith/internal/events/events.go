// Package events — async messaging between services via NATS JetStream.
//
// State-changing services (player, team) publish events; interested services
// (leaderboard) subscribe. When NATS_URL is not configured (monolith mode,
// unit tests) the NoopPublisher is used and publishing is a no-op.
package events

import (
	"context"
	"encoding/json"
	"time"
)

// Subjects published on the ARK8DE stream.
const (
	SubjectTeamPointsUpdated     = "team.points_updated"
	SubjectTeamMembershipChanged = "team.membership_changed"
	SubjectPlayerGearUpdated     = "player.gear_updated"
)

// TeamPointsUpdated is emitted when a moderator adjusts a team's Arkade points.
type TeamPointsUpdated struct {
	TeamID    string    `json:"team_id"`
	Delta     int       `json:"delta"`
	Reason    string    `json:"reason"`
	ChangedBy string    `json:"changed_by"`
	At        time.Time `json:"at"`
}

// TeamMembershipChanged is emitted when a player joins or is removed from a team.
type TeamMembershipChanged struct {
	TeamID   string    `json:"team_id"`
	PlayerID string    `json:"player_id"`
	Action   string    `json:"action"` // "joined" or "removed"
	At       time.Time `json:"at"`
}

// PlayerGearUpdated is emitted when a player changes their gear selection.
type PlayerGearUpdated struct {
	PlayerID string    `json:"player_id"`
	At       time.Time `json:"at"`
}

// Publisher is the interface services use to emit events.
// Production uses NATSPublisher; the monolith and tests use NoopPublisher.
type Publisher interface {
	Publish(ctx context.Context, subject string, payload any) error
}

// NoopPublisher discards all events. Used when NATS is not configured.
type NoopPublisher struct{}

func (NoopPublisher) Publish(context.Context, string, any) error { return nil }

// marshal serializes a payload for the wire.
func marshal(payload any) ([]byte, error) {
	return json.Marshal(payload)
}
