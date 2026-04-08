-- Migration: 000006_create_player_skill_allocations (UP)
-- Creates the player_skill_allocations join table.
--
-- This table records which skills each player has allocated.
-- It is a many-to-many relationship: a player can allocate many skills,
-- and the same skill can be allocated by many players.
--
-- The application enforces:
--   1. Players can only allocate skills matching their class_role
--   2. The total cost of all allocated skills cannot exceed skill_points_total
-- These rules are enforced in the service layer (Phase 1), not the DB.

CREATE TABLE player_skill_allocations (
    -- The player who allocated this skill.
    player_id UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,

    -- The skill that was allocated.
    -- ON DELETE CASCADE: if a skill is removed from the game (unlikely but possible),
    -- all player allocations for it are also removed.
    skill_id UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,

    -- When the player allocated this skill.
    allocated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Composite primary key prevents a player from allocating the same skill twice.
    PRIMARY KEY (player_id, skill_id)
);
