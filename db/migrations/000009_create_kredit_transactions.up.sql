-- Migration: 000009_create_kredit_transactions (UP)
-- Creates the kredit_transactions table.
--
-- Kredits are the in-game currency. Every change to a player's kredit
-- balance is recorded as an immutable transaction — we never just UPDATE
-- the balance. This gives a full audit trail of who sent what to whom and why.
--
-- KREDIT FLOWS:
--   Moderator grant:      from_player_id = NULL, to_player_id = recipient
--   Player-to-player:     from_player_id = sender, to_player_id = recipient
--
-- The players.kredits column stores the running balance (updated atomically
-- with the transaction INSERT in the application layer).

CREATE TABLE kredit_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- from_player_id is nullable: NULL means the kredits were granted by a
    -- moderator (not transferred from another player's balance).
    -- ON DELETE SET NULL: if the sending player is deleted, the transaction
    -- record is preserved but the sender becomes anonymous (NULL).
    from_player_id UUID REFERENCES players(id) ON DELETE SET NULL,

    -- to_player_id is the recipient. NOT NULL — every transaction must have
    -- a recipient.
    to_player_id UUID NOT NULL REFERENCES players(id),

    -- The number of kredits transferred. Must be positive — transactions are
    -- always a positive amount; the direction is implied by from/to.
    amount INTEGER NOT NULL
        CHECK (amount > 0),

    -- Optional free-text reason for the transaction.
    -- e.g. "prize for winning the round", "payment for healing"
    note TEXT,

    -- created_by records which player (or moderator) initiated the transaction.
    -- This is always set, even for moderator grants (created_by = moderator's ID).
    created_by UUID NOT NULL REFERENCES players(id),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
