-- Migration: 000012_add_skill_bonuses (DOWN)
-- Removes hp_bonus and armor_bonus from the skills table,
-- reverting the schema to its state before migration 000012.
ALTER TABLE skills
    DROP COLUMN hp_bonus,
    DROP COLUMN armor_bonus;
