-- Migration: 000004_create_join_requests (UP)
-- Creates the join_requests table.
--
-- When a player wants to join a team, they submit a join request.
-- The team owner then accepts or rejects it.
-- Accepted requests result in a new row in team_members.
--
-- Workflow:
--   1. Player sends a join request (status = 'pending')
--   2. Team owner accepts (status = 'accepted', player added to team_members)
--      OR rejects (status = 'rejected')

CREATE TABLE join_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- The team this request is for.
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,

    -- The player who wants to join.
    player_id UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,

    -- status tracks the lifecycle of the request.
    -- 'pending'  — submitted, awaiting team owner's decision
    -- 'accepted' — team owner accepted; player was added to team_members
    -- 'rejected' — team owner rejected; player may try again later
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'rejected')),

    -- requested_at records when the request was submitted.
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- resolved_at records when the team owner accepted or rejected.
    -- Nullable: NULL means the request is still pending.
    resolved_at TIMESTAMPTZ
);

-- Partial unique index: a player can have at most one PENDING request per team.
-- The WHERE clause limits the index to only rows where status = 'pending',
-- so a player CAN have multiple rejected/accepted requests for the same team
-- (e.g. they were rejected and tried again) — only one pending at a time.
-- This is a more nuanced constraint than a simple UNIQUE(team_id, player_id).
CREATE UNIQUE INDEX idx_join_requests_one_pending_per_player_team
    ON join_requests (team_id, player_id)
    WHERE status = 'pending';
