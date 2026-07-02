-- Migration: 000014_archetype_redesign (DOWN)
-- Restores the placeholder tank/dps/healer/support model.
-- Skill/gear DATA from before this migration is NOT restored (skills were
-- seeded by the seed script; gear reverts to the original 4-item catalog).

-- 4. Gear: back to the original catalog
DELETE FROM player_gear;
DELETE FROM gear_types;
ALTER TABLE gear_types DROP COLUMN restricted_to;
INSERT INTO gear_types (name, gear_point_cost) VALUES
    ('sword',          2),
    ('bow_and_arrow',  3),
    ('spear',          4),
    ('shield',         5);

-- 3. Skills: back to the flat SP-cost model
DROP TABLE player_skill_allocations;
DROP TABLE skills;

CREATE TABLE skills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    class_role VARCHAR(20) NOT NULL
        CHECK (class_role IN ('tank', 'dps', 'healer', 'support')),
    cost_skill_points INTEGER NOT NULL
        CHECK (cost_skill_points > 0),
    effect_description TEXT NOT NULL,
    effect_type VARCHAR(20) NOT NULL
        CHECK (effect_type IN ('passive', 'active')),
    hp_bonus    INTEGER NOT NULL DEFAULT 0 CHECK (hp_bonus    >= 0),
    armor_bonus INTEGER NOT NULL DEFAULT 0 CHECK (armor_bonus >= 0)
);

CREATE TABLE player_skill_allocations (
    player_id    UUID NOT NULL REFERENCES players (id) ON DELETE CASCADE,
    skill_id     UUID NOT NULL REFERENCES skills (id)  ON DELETE CASCADE,
    allocated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (player_id, skill_id)
);

-- 2. Players: old class list, restore SP budget, drop level
UPDATE players SET class_role = NULL;
ALTER TABLE players DROP CONSTRAINT players_class_role_check;
ALTER TABLE players ADD CONSTRAINT players_class_role_check
    CHECK (class_role IN ('tank', 'dps', 'healer', 'support'));
ALTER TABLE players DROP COLUMN level;
ALTER TABLE players ADD COLUMN skill_points_total INTEGER NOT NULL DEFAULT 20;

-- 1. Archetypes
DROP TABLE archetypes;
