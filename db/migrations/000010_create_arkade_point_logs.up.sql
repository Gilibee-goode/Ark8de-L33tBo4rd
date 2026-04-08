-- Migration: 000010_create_arkade_point_logs (UP)
-- Creates the arkade_point_logs table.
--
-- Arkade points are the competition score — moderators assign them to teams
-- based on performance in the arena. Every change is logged here for
-- accountability and history.
--
-- Like kredit_transactions, this table is immutable — we never update rows,
-- only insert new ones. The teams.arkade_points column stores the current total.
--
-- delta can be positive (points awarded) or negative (points deducted).

CREATE TABLE arkade_point_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- The team that received (or lost) the points.
    -- ON DELETE CASCADE: if the team is deleted, its point history goes with it.
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,

    -- The moderator or admin who made the change.
    changed_by UUID NOT NULL REFERENCES players(id),

    -- delta is positive for points awarded, negative for points deducted.
    -- e.g. +10 for winning a round, -5 for a rule violation.
    -- Not constrained to positive: moderators can deduct points.
    delta INTEGER NOT NULL,

    -- Optional explanation of why points were awarded or deducted.
    -- Displayed in the audit log on the moderator panel.
    reason TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
