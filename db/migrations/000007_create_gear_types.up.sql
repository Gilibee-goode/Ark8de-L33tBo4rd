-- Migration: 000007_create_gear_types (UP)
-- Creates the gear_types reference table and seeds the 4 gear types.
--
-- Gear types are the static set of weapons/equipment available in the game.
-- Unlike player data (which changes), gear types are reference data —
-- they are defined once and don't change during normal gameplay.
-- We store them in the database (rather than hardcoding in Go) so they
-- can be displayed in queries without JOIN lookups into application code.
--
-- GEAR TYPES AND COSTS:
--   Sword         — 2 gear points  (cheap, basic weapon)
--   Bow & Arrow   — 3 gear points  (ranged, moderate cost)
--   Spear         — 4 gear points  (reach weapon, higher cost)
--   Shield        — 5 gear points  (defensive, most expensive)

CREATE TABLE gear_types (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- The gear's display name. Must be unique — two gear types can't share a name.
    name VARCHAR(50) UNIQUE NOT NULL,

    -- How many of the team's shared gear points this item costs to equip.
    -- Must be positive: free gear would make the pool meaningless.
    gear_point_cost INTEGER NOT NULL
        CHECK (gear_point_cost > 0)
);

-- Seed the 4 gear types defined by the game rules.
-- We use INSERT in the UP migration so that the data is part of the
-- migration history — rolling back this migration also removes the data.
-- gen_random_uuid() generates a new UUID for each row.
INSERT INTO gear_types (name, gear_point_cost) VALUES
    ('sword',          2),
    ('bow_and_arrow',  3),
    ('spear',          4),
    ('shield',         5);
