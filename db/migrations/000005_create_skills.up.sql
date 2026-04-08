-- Migration: 000005_create_skills (UP)
-- Creates the skills table.
--
-- Skills are game abilities that players can unlock by spending skill points.
-- Each skill belongs to a specific class role (tank/dps/healer/support).
-- The seed script (Phase 1) will populate this table with actual skill data.
--
-- Skill point budget:
--   - Each player has skill_points_total (default 20)
--   - Each skill has a cost_skill_points
--   - Players can allocate skills as long as their remaining budget >= cost
--   - skill_points_remaining is derived: total - sum(allocated costs)

CREATE TABLE skills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Human-readable skill name, e.g. "Iron Fortress", "Rapid Strike"
    name VARCHAR(100) NOT NULL,

    -- Optional longer description of the skill's lore or flavour text.
    description TEXT,

    -- The class role this skill is available to.
    -- Players can only allocate skills matching their own class_role.
    class_role VARCHAR(20) NOT NULL
        CHECK (class_role IN ('tank', 'dps', 'healer', 'support')),

    -- The skill point cost to allocate this skill.
    -- Must be positive: a free skill would allow infinite allocation.
    cost_skill_points INTEGER NOT NULL
        CHECK (cost_skill_points > 0),

    -- effect_description is a short game-mechanic description,
    -- e.g. "+15 armor" or "restore 20 HP to nearest ally once per round".
    effect_description TEXT NOT NULL,

    -- effect_type categorises how the skill works in the game.
    -- 'passive' — always active (e.g. "+10 HP permanently")
    -- 'active'  — triggered by the player during the game (e.g. "use once per round")
    effect_type VARCHAR(20) NOT NULL
        CHECK (effect_type IN ('passive', 'active'))
);
