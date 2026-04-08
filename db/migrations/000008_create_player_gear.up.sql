-- Migration: 000008_create_player_gear (UP)
-- Creates the player_gear join table.
--
-- This table records which gear a player has selected.
-- A player can have multiple gear items — limited by the team's shared
-- gear_points_total budget (enforced in the application layer).
--
-- GEAR POOL MECHANICS:
--   team.gear_points_total starts at 12.
--   Each team member's selected gear costs are summed and subtracted.
--   The pool can go negative if members overspend — this is intentional
--   and is displayed as a warning. Players must reduce gear to fix it.

CREATE TABLE player_gear (
    -- The player who selected this gear.
    player_id UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,

    -- The type of gear selected.
    -- No ON DELETE CASCADE here — gear types are reference data and should
    -- not be deleted while players have them equipped.
    gear_type_id UUID NOT NULL REFERENCES gear_types(id),

    -- When the player selected this gear item.
    selected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Composite primary key: a player can equip each gear type at most once.
    -- You can't equip two swords — that would double-count the gear cost.
    PRIMARY KEY (player_id, gear_type_id)
);
