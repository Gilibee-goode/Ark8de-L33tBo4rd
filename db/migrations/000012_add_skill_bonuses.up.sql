-- Migration: 000012_add_skill_bonuses (UP)
-- Adds hp_bonus and armor_bonus columns to the skills table so that
-- allocated skills can grant numerical stat increases to players.
--
-- Without these columns the stats endpoint cannot compute health_points
-- and armor_points from skill allocations — it would only have the
-- base values from class_role with no way to add skill bonuses.
--
-- Both columns default to 0 so existing rows (if any) are not broken.
ALTER TABLE skills
    ADD COLUMN hp_bonus    INTEGER NOT NULL DEFAULT 0
        CHECK (hp_bonus    >= 0),  -- bonuses cannot be negative
    ADD COLUMN armor_bonus INTEGER NOT NULL DEFAULT 0
        CHECK (armor_bonus >= 0);
