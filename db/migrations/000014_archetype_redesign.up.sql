-- Migration: 000014_archetype_redesign (UP)
-- Replaces the placeholder class/skill/gear model with the real Ark8de
-- archetype mechanics (source: "ארקייד- מכאניקת ארכיטיפים.xlsx").
--
--   * 6 archetypes (smartass, ninja, psycho, hacker, merkava, kommando)
--     with passive traits, replacing tank/dps/healer/support
--   * Skill trees: 2 branches (blue/red) x 3 tiers per archetype.
--     A player holds at most ONE skill per tier (blue or red), and can
--     only take skills up to their level (1-3). The skill-point budget
--     mechanic is retired.
--   * Weapon catalog from the price sheet, with class restrictions.
--     Armor is free and untracked (restrictions live in passive text).

-- ---------------------------------------------------------------------
-- 1. Archetypes reference table
-- ---------------------------------------------------------------------
CREATE TABLE archetypes (
    class_role          VARCHAR(20) PRIMARY KEY,
    display_name        VARCHAR(50) NOT NULL,
    passive_description TEXT        NOT NULL,
    -- Permanent HP granted by the archetype's passive (merkava: +1).
    hp_bonus            INTEGER     NOT NULL DEFAULT 0
);

INSERT INTO archetypes (class_role, display_name, passive_description, hp_bonus) VALUES
    ('smartass', 'Smartass', 'Cannot use heavy armor. Cannot use a shield.',                                     0),
    ('ninja',    'Ninja',    'Cannot use heavy armor. Cannot use a shield.',                                     0),
    ('psycho',   'Psycho',   'Cannot use heavy or medium armor. Cannot use pole weapons or long weapons.',       0),
    ('hacker',   'Hacker',   'Cannot use heavy or medium armor. Cannot use pole weapons or long weapons.',       0),
    ('merkava',  'Merkava',  'Cannot use light armor. Grants +1 HP.',                                            1),
    ('kommando', 'Kommando', 'No equipment restrictions.',                                                       0);

-- ---------------------------------------------------------------------
-- 2. Players: new class list, level, drop the skill-point budget
-- ---------------------------------------------------------------------
-- Old classes are meaningless in the new model — players re-pick.
UPDATE players SET class_role = NULL;

ALTER TABLE players DROP CONSTRAINT players_class_role_check;
ALTER TABLE players ADD CONSTRAINT players_class_role_check
    CHECK (class_role IN ('smartass', 'ninja', 'psycho', 'hacker', 'merkava', 'kommando'));

-- Level gates skill tiers: a level-N player may hold skills of tier <= N.
-- Only tiers 1-3 are implemented for now.
ALTER TABLE players ADD COLUMN level INTEGER NOT NULL DEFAULT 1
    CHECK (level BETWEEN 1 AND 3);

ALTER TABLE players DROP COLUMN skill_points_total;

-- ---------------------------------------------------------------------
-- 3. Skills: rebuild as branch/tier trees (drops all old skills/allocations)
-- ---------------------------------------------------------------------
DROP TABLE player_skill_allocations;
DROP TABLE skills;

CREATE TABLE skills (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    class_role  VARCHAR(20) NOT NULL REFERENCES archetypes (class_role),
    branch      VARCHAR(10) NOT NULL CHECK (branch IN ('blue', 'red')),
    tier        INTEGER     NOT NULL CHECK (tier BETWEEN 1 AND 3),
    name        VARCHAR(100) NOT NULL,
    description TEXT        NOT NULL,
    -- Permanent HP granted by holding this skill (e.g. merkava's Fridge).
    hp_bonus    INTEGER     NOT NULL DEFAULT 0,
    -- Extra shared gear points granted to the holder's TEAM (hacker's Grid is Good).
    team_gear_bonus INTEGER NOT NULL DEFAULT 0,
    -- One skill per branch slot per archetype.
    UNIQUE (class_role, branch, tier)
);

CREATE TABLE player_skill_allocations (
    player_id    UUID NOT NULL REFERENCES players (id) ON DELETE CASCADE,
    skill_id     UUID NOT NULL REFERENCES skills (id)  ON DELETE CASCADE,
    allocated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (player_id, skill_id)
);

-- Skill content, translated from the Hebrew archetype sheet (rows 4-6 = tiers 1-3).
INSERT INTO skills (class_role, branch, tier, name, description, hp_bonus, team_gear_bonus) VALUES
-- Smartass
('smartass', 'blue', 1, 'Cheats',            'Can cast Freeze twice.', 0, 0),
('smartass', 'red',  1, 'The Coward',        'Cross your arms over your chest (going off-play) and return immediately to base.', 0, 0),
('smartass', 'blue', 2, 'Red Bull Shot',     'Heal 1 HP to a teammate who is not dying. Can be used four times.', 0, 0),
('smartass', 'red',  2, 'Playing Dirty',     'Weaken a character while in the Nexus: they start their next fight with 3 fewer HP (minimum 1) and cannot recover them until after their first dying state. Can only be used before a battle. The weakened player must be informed off-play or via an available master. A character can be affected by Playing Dirty only once per battle.', 0, 0),
('smartass', 'blue', 3, 'Take a Shpitzur!',  'Grant a teammate a one-time Freeze or a one-time Mass Knockback. Can be used twice.', 0, 0),
('smartass', 'red',  3, 'Bullshitsu',        'Instead of taking damage, fake a dying state. The Smartass cannot be killed while faking. Can only be activated at the moment of being hit.', 0, 0),
-- Ninja
('ninja', 'blue', 1, 'The Elusive',             'Ignore a hit of your choice. Can be used twice.', 0, 0),
('ninja', 'red',  1, 'The Pusher',              'Twice per game: cast Knockback on an enemy immediately after a successful hit on that enemy with a non-ranged weapon.', 0, 0),
('ninja', 'blue', 2, 'Close-Range Intimidator', 'Twice per game: cast Fear on an enemy immediately after a successful hit on that enemy with a non-ranged weapon.', 0, 0),
('ninja', 'red',  2, 'The Coward',              'Cross your arms over your chest (going off-play) and return immediately to base.', 0, 0),
('ninja', 'blue', 3, 'Ninjitsu',                'Hits from ranged weapons and power balls on the arm, from palm to elbow, do not reduce HP.', 0, 0),
('ninja', 'red',  3, 'It''s Not Personal',      'Twice per game: cast a short 30-second Freeze on an enemy immediately after a successful hit on that enemy with a non-ranged weapon.', 0, 0),
-- Psycho
('psycho', 'blue', 1, 'Basic Psychosis',        'Can cast Knockback twice.', 0, 0),
('psycho', 'red',  1, 'Rechargeable Batteries', 'Carry 4 extra power balls (if power balls are chosen as a weapon). The character can return to base and rest for 30 seconds; after resting, restore up to 4 power balls.', 0, 0),
('psycho', 'blue', 2, 'Psychic Armor',          'Apply psychic armor to yourself and one other character, granting +1 HP.', 0, 0),
('psycho', 'red',  2, 'The Intimidator',        'Can cast Fear twice.', 0, 0),
('psycho', 'blue', 3, 'Advanced Psychosis',     'Can cast Mass Knockback twice.', 0, 0),
('psycho', 'red',  3, 'Flipping Out',           'Instead of dying, the Psycho flips out: immune to Fear and Knockback; cannot use abilities, ranged weapons, or a shield; can use any melee weapon; has 7 HP; cannot heal or raise HP above 7. On the next dying state the Psycho explodes and dies. A flipped Psycho who does not fight for over a minute explodes and dies.', 0, 0),
-- Hacker
('hacker', 'blue', 1, 'Cheats',                 'Can cast Freeze twice.', 0, 0),
('hacker', 'red',  1, 'Rechargeable Batteries', 'Carry 4 extra power balls (if power balls are chosen as a weapon). The character can return to base and rest for 30 seconds; after resting, restore up to 4 power balls.', 0, 0),
('hacker', 'blue', 2, 'It''s a Feature',        'Neutralize a negative effect on a teammate (Freeze, Fear, Disable...). Can be used twice. Cannot cancel Playing Dirty. A successful neutralization lets the Hacker immediately cast the same effect on another character; if not cast within a few seconds, the cast is lost.', 0, 0),
('hacker', 'red',  2, 'It''s a Bug',            'Disable an opponent''s combat equipment until the opponent returns to base and fixes the bug for 10 seconds.', 0, 0),
('hacker', 'blue', 3, 'Grid is Good',           'Grants the Hacker''s team 4 extra equipment points.', 0, 4),
('hacker', 'red',  3, 'God Mode',               'Cast God Mode on yourself or a teammate: immune to ranged hits, but HP is locked at 1 while active. Can be used twice per game; only one God Mode can be active at a time.', 0, 0),
-- Merkava
('merkava', 'blue', 1, 'Fridge',                  'Grants +1 HP.', 1, 0),
('merkava', 'red',  1, 'Nailed to the Floor',     'Immune to Knockback and Mass Knockback.', 0, 0),
('merkava', 'blue', 2, 'Windbreaker',             'Ranged weapons deal 1 HP of damage instead of 2.', 0, 0),
('merkava', 'red',  2, 'Close-Range Intimidator', 'Twice per game: cast Fear on an enemy immediately after a successful hit on that enemy with a non-ranged weapon.', 0, 0),
('merkava', 'blue', 3, 'Too Dumb',                'Ignore one effect of your choice. Can be used twice.', 0, 0),
('merkava', 'red',  3, 'Brain Glitch',            'The first time the character enters a dying state in a fight, they return to 2 HP instead. The second time they enter a dying state, they die immediately.', 0, 0),
-- Kommando
('kommando', 'blue', 1, 'Psychic Armor',              'Apply psychic armor to yourself and one other character, granting +1 HP.', 0, 0),
('kommando', 'red',  1, 'The Pusher',                 'Twice per game: cast Knockback on an enemy immediately after a successful hit on that enemy with a non-ranged weapon.', 0, 0),
('kommando', 'blue', 2, 'Adrenaline Shot',            'Once per game: restore a dying character to 2 HP.', 0, 0),
('kommando', 'red',  2, 'Dramatic Speech',            'Heal 1 HP to a teammate who is not dying. Can be used four times.', 0, 0),
('kommando', 'blue', 3, 'Even More Psychic Armor',    'Apply psychic armor (+1 HP) to yourself and a friend. If you can already cast Psychic Armor, this lets you cast it on the entire team (in addition to yourself).', 0, 0),
('kommando', 'red',  3, 'Friend of the Quartermaster', 'Doubles the team''s ammunition.', 0, 0);

-- ---------------------------------------------------------------------
-- 4. Gear: weapon catalog from the price sheet, with class restrictions
-- ---------------------------------------------------------------------
DELETE FROM player_gear;
DELETE FROM gear_types;

-- restricted_to: NULL = any class may equip it; otherwise only the listed
-- classes may. (Psycho/Hacker cannot use pole/long weapons; Smartass/Ninja
-- cannot use a shield; power balls are Psycho/Hacker ammo.)
ALTER TABLE gear_types ADD COLUMN restricted_to TEXT[];

INSERT INTO gear_types (name, gear_point_cost, restricted_to) VALUES
    ('short_weapon',       1, NULL),
    ('dagger',             1, NULL),  -- a.k.a. "docker"
    ('long_weapon',        2, ARRAY['smartass', 'ninja', 'merkava', 'kommando']),
    ('spear',              3, ARRAY['smartass', 'ninja', 'merkava', 'kommando']),  -- "long docker"
    ('bow',                4, NULL),
    ('pistol',             4, NULL),
    ('shield',             4, ARRAY['psycho', 'hacker', 'merkava', 'kommando']),   -- "firewall"
    ('power_balls_x4',     1, ARRAY['psycho', 'hacker']);
