// Package player manages everything that belongs to an individual player:
// their archetype (class), level, computed combat stats, skill allocations,
// gear selections, and Kredit balance.
//
// Layer files in this package:
//   - model.go      — structs, constants, request/response types
//   - repository.go — SQL queries
//   - service.go    — business logic (tier/branch rules, gear restrictions)
//   - handler.go    — HTTP request parsing and response writing
package player

import "time"

// ---------------------------------------------------------------------------
// Game constants
// ---------------------------------------------------------------------------

// BaseHP is the uniform starting HP (נק"פ) for every archetype.
// Archetype passives (merkava: +1) and skills (Fridge: +1) add on top.
const BaseHP = 3

// MaxLevel caps player levels — skill tiers 1–3 are implemented so far.
const MaxLevel = 3

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// Archetype is one of the six playable classes, with its passive traits.
type Archetype struct {
	ClassRole          string `json:"class_role"`
	DisplayName        string `json:"display_name"`
	PassiveDescription string `json:"passive_description"`
	HPBonus            int    `json:"hp_bonus"` // permanent HP from the passive
}

// Skill is one node in an archetype's skill tree.
// Each archetype has two branches (blue/red) with one skill per tier (1–3).
type Skill struct {
	ID            string `json:"id"`
	ClassRole     string `json:"class_role"`
	Branch        string `json:"branch"` // "blue" or "red"
	Tier          int    `json:"tier"`   // 1–3; requires player level >= tier
	Name          string `json:"name"`
	Description   string `json:"description"`
	HPBonus       int    `json:"hp_bonus"`        // permanent HP granted by holding this skill
	TeamGearBonus int    `json:"team_gear_bonus"` // extra shared gear points for the holder's team
}

// GearType represents one weapon in the catalog.
// RestrictedTo lists the classes allowed to equip it; nil = everyone.
type GearType struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	GearPointCost int      `json:"gear_point_cost"`
	RestrictedTo  []string `json:"restricted_to,omitempty"`
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
	Level           int     `json:"level"`
}

// StatsResponse contains a player's computed combat stats.
// All values are derived at read time — nothing here is stored in the DB.
type StatsResponse struct {
	ClassRole      *string `json:"class_role"` // nil if class not yet chosen
	Level          int     `json:"level"`
	HealthPoints   int     `json:"health_points"` // BaseHP + passive + skill bonuses
	GearPointsUsed int     `json:"gear_points_used"`
}

// SkillsResponse is the payload for GET /players/me/skills.
type SkillsResponse struct {
	AllocatedSkills []Skill `json:"allocated_skills"`
	Level           int     `json:"level"` // highest tier the player may hold
}

// GearResponse is the payload for GET /players/me/gear.
// It includes both the player's selections and the team's pool summary.
type GearResponse struct {
	SelectedGear         []GearType `json:"selected_gear"`
	GearPointsUsedByMe   int        `json:"gear_points_used_by_me"`
	TeamGearPointsTotal  int        `json:"team_gear_points_total"` // 0 if not on a team
	TeamGearPointsUsed   int        `json:"team_gear_points_used"`  // 0 if not on a team
	TeamGearPoolNegative bool       `json:"team_gear_pool_negative"`
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
	ClassRole string `json:"class_role"` // smartass, ninja, psycho, hacker, merkava, kommando
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

// SetLevelRequest is the body for PUT /players/:id/level (moderator only).
type SetLevelRequest struct {
	Level int `json:"level"`
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
