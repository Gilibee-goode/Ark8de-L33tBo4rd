-- Migration: 000003_create_team_members (UP)
-- Creates the team_members join table.
--
-- This is a many-to-many relationship: a player can be on one team,
-- and a team has many players. The join table has a row for each
-- (team, player) pair.
--
-- Note: The game rules allow a player to be on at most one team,
-- but this constraint is enforced at the application layer (Phase 1)
-- rather than in the database, to allow future flexibility.

CREATE TABLE team_members (
    -- team_id references the team this membership belongs to.
    -- ON DELETE CASCADE: if the team is deleted, all its member rows
    -- are automatically deleted — no orphaned memberships.
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,

    -- player_id references the player who is a member.
    -- ON DELETE CASCADE: if a player is deleted, their membership is removed.
    player_id UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,

    -- joined_at records when the player joined the team.
    -- Useful for displaying "joined X days ago" on the roster.
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Composite primary key: the combination of (team_id, player_id) must be unique.
    -- This prevents a player from appearing twice on the same team.
    PRIMARY KEY (team_id, player_id)
);
