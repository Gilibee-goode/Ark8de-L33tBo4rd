-- Migration: 000011_create_leaderboard_entries (UP)
-- Creates the leaderboard_entries table.
--
-- This is a read-optimised denormalised cache of team data for the leaderboard.
-- Instead of joining teams + team_members every time someone loads the leaderboard,
-- we maintain a pre-computed snapshot here.
--
-- WHY DENORMALISE?
--   The leaderboard is the most frequently accessed page — potentially 100s of reads
--   per second during a live event. Joining multiple tables on every read would be
--   slow. Caching the results here means a single SELECT with no JOINs.
--
-- The application (Phase 1) is responsible for keeping this table in sync:
--   - When arkade_points changes on a team → update arkade_points here
--   - When a member joins/leaves → update member_count here
--   - Rank is recalculated and stored here after every points change
--
-- In Phase 3+ this will be maintained by the leaderboard-service via events.

CREATE TABLE leaderboard_entries (
    -- team_id is both the PK and the FK to teams.
    -- One row per team — if the team is deleted, its leaderboard entry is too.
    team_id UUID PRIMARY KEY REFERENCES teams(id) ON DELETE CASCADE,

    -- Cached team fields — duplicated here to avoid JOINs on read.
    -- These must be kept in sync when the source data changes.
    team_name VARCHAR(100) NOT NULL,
    team_tag VARCHAR(10) NOT NULL,
    team_logo_url VARCHAR(500), -- nullable: teams may not have a logo

    -- The team's current Arkade point total — the primary sort key for rankings.
    arkade_points INTEGER NOT NULL DEFAULT 0,

    -- rank is the team's position on the leaderboard (1 = first place).
    -- Nullable: recalculated after each points change; NULL means "not yet ranked".
    rank INTEGER,

    -- member_count is the number of players currently on the team.
    -- Cached here to avoid a COUNT(*) JOIN on every leaderboard load.
    member_count INTEGER NOT NULL DEFAULT 0,

    -- last_updated_at tracks when this row was last refreshed.
    -- Useful for debugging stale data issues.
    last_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
