-- Migration: 000002_create_teams (UP)
-- Creates the teams table.
-- A team is a group of players competing together in the arena.
-- Each team has an owner (a player with the team_owner role),
-- an Arkade points balance (set by moderators), and a gear point pool
-- shared among all team members.

CREATE TABLE teams (
    -- UUID primary key, same pattern as players.
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- name is the team's full display name shown on the leaderboard.
    -- Must be unique across all teams.
    name VARCHAR(100) UNIQUE NOT NULL,

    -- tag is the short identifier shown next to player names (like [ARK]).
    -- Limited to 10 characters; must be unique.
    tag VARCHAR(10) UNIQUE NOT NULL,

    -- owner_id references the player who created and manages this team.
    -- When a player creates a team, their role is promoted to 'team_owner'.
    -- NOT NULL: every team must have an owner.
    -- ON DELETE RESTRICT (default) prevents deleting a player who owns a team —
    -- ownership must be transferred first.
    owner_id UUID NOT NULL REFERENCES players(id),

    -- logo_url is the team's uploaded logo image URL.
    -- Nullable: teams don't need a logo to be created.
    logo_url VARCHAR(500),

    -- arkade_points is the team's score in the competition.
    -- Moderators assign (or subtract) points using the arkade_point_logs table.
    -- This column stores the running total.
    arkade_points INTEGER NOT NULL DEFAULT 0,

    -- gear_points_total is the team's shared gear point budget.
    -- Members spend from this pool when selecting gear.
    -- The pool CAN go negative if members overspend — this is intentional
    -- and displayed as a warning on the team's page.
    -- Default is 12 per the game rules.
    gear_points_total INTEGER NOT NULL DEFAULT 12,

    -- is_locked_in marks whether the team has finalised their roster
    -- and gear/skill selections for the competition.
    -- Once locked in, the team owner cannot make further changes
    -- (enforced at the application layer, not the DB).
    is_locked_in BOOLEAN NOT NULL DEFAULT FALSE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
