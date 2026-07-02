// Package player manages everything that belongs to an individual player:
// their class role, computed combat stats, skill allocations, gear selections,
// and Kredit balance.
//
// This package owns no HTTP-specific types — it works entirely in domain terms
// (players, skills, gear) and lets handler.go translate to/from HTTP.
//
// Layer files in this package:
//   - model.go      — structs, constants, request/response types
//   - repository.go — SQL queries
//   - service.go    — business logic (stat computation, budget enforcement)
//   - handler.go    — HTTP request parsing and response writing
package player

import "time"

// ---------------------------------------------------------------------------
// Base stats per class role
// ---------------------------------------------------------------------------

// classBaseHP maps each class role to its base health points.
// These are the starting HP before any skill bonuses are applied.
// The map key must exactly match the class_role values stored in the DB.
var classBaseHP = map[string]int{
	"tank":    150,
	"dps":     80,
	"healer":  100,
	"support": 90,
}

// classBaseArmor maps each class role to its base armor points.
var classBaseArmor = map[string]int{
	"tank":    30,
	"dps":     10,
	"healer":  15,
	"support": 20,
}

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// Skill represents one skill that players can allocate.
// Skills are seeded at startup and are read-only during normal gameplay.
type Skill struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	ClassRole       string `json:"class_role"`       // which class can use this skill
	CostSkillPoints int    `json:"cost_skill_points"` // how many skill points it costs
	EffectDesc      string `json:"effect_description"`
	EffectType      string `json:"effect_type"` // "passive" or "active"
	HPBonus         int    `json:"hp_bonus"`
	ArmorBonus      int    `json:"armor_bonus"`
}

// GearType represents one type of equipment (sword, shield, etc.).
// These are seeded at startup via migration 000007 and are read-only.
type GearType struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	GearPointCost int    `json:"gear_point_cost"`
}

// KreditTransaction represents one entry in the Kredit audit log.
// The log is append-only — transactions are never deleted or modified.
type KreditTransaction struct {
	ID           string    `json:"id"`
	FromPlayerID *string   `json:"from_player_id"` // nil = moderator grant
	ToPlayerID   string    `json:"to_player_id"`
	Amount       int       `json:"amount"`
	Note         *string   `json:"note"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

// PublicPlayerResponse is what anyone can see about a player — no sensitive data.
type PublicPlayerResponse struct {
	ID              string  `json:"id"`
	Username        string  `json:"username"`
	Role            string  `json:"role"`
	ProfilePhotoURL *string `json:"profile_photo_url"`
	ClassRole       *string `json:"class_role"`
}

// StatsResponse contains a player's computed combat stats.
// All values are derived at read time — nothing here is stored in the DB.
type StatsResponse struct {
	ClassRole             *string `json:"class_role"`              // nil if class not yet chosen
	HealthPoints          int     `json:"health_points"`           // base + skill bonuses
	ArmorPoints           int     `json:"armor_points"`            // base + skill bonuses
	SkillPointsTotal      int     `json:"skill_points_total"`
	SkillPointsRemaining  int     `json:"skill_points_remaining"`  // total - spent
	GearPointsUsed        int     `json:"gear_points_used"`        // sum of my gear costs
}

// SkillsResponse is the payload for GET /players/me/skills.
type SkillsResponse struct {
	AllocatedSkills      []Skill `json:"allocated_skills"`
	SkillPointsTotal     int     `json:"skill_points_total"`
	SkillPointsRemaining int     `json:"skill_points_remaining"`
}

// GearResponse is the payload for GET /players/me/gear.
// It includes both the player's selections and the team's pool summary.
type GearResponse struct {
	SelectedGear          []GearType `json:"selected_gear"`
	GearPointsUsedByMe    int        `json:"gear_points_used_by_me"`
	TeamGearPointsTotal   int        `json:"team_gear_points_total"`   // 0 if not on a team
	TeamGearPointsUsed    int        `json:"team_gear_points_used"`    // 0 if not on a team
	TeamGearPoolNegative  bool       `json:"team_gear_pool_negative"`  // true = team is over budget
}

// KreditsResponse is the payload for GET /players/me/kredits.
type KreditsResponse struct {
	Balance      int                 `json:"balance"`
	Transactions []KreditTransaction `json:"transactions"`
}

// ---------------------------------------------------------------------------
// Request types
// ---------------------------------------------------------------------------

// SetClassRequest is the body for PUT /players/me/class.
type SetClassRequest struct {
	ClassRole string `json:"class_role"` // "tank", "dps", "healer", "support"
}

// SetSkillsRequest is the body for PUT /players/me/skills.
// The list replaces the player's entire skill allocation in one operation.
type SetSkillsRequest struct {
	SkillIDs []string `json:"skill_ids"`
}

// SetGearRequest is the body for PUT /players/me/gear.
// The list replaces the player's entire gear selection in one operation.
type SetGearRequest struct {
	GearTypeIDs []string `json:"gear_type_ids"`
}

// GrantKreditsRequest is the body for POST /players/:id/kredits (moderator only).
type GrantKreditsRequest struct {
	Amount int    `json:"amount"`
	Note   string `json:"note"`
}

// TransferKreditsRequest is the body for POST /players/me/kredits/transfer.
type TransferKreditsRequest struct {
	ToPlayerID string `json:"to_player_id"`
	Amount     int    `json:"amount"`
	Note       string `json:"note"`
}
