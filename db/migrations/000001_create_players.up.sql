-- Migration: 000001_create_players (UP)
-- Creates the players table — the central entity of the entire game.
-- Every user of the system is a player, whether they are a regular player,
-- team owner, moderator, or admin. Role is stored on this table.
--
-- To roll this back, run: go run ./cmd/migrate down
-- The corresponding rollback is in 000001_create_players.down.sql

-- Enable the pgcrypto extension so we can use gen_random_uuid() for primary keys.
-- Extensions only need to be enabled once per database; subsequent migrations
-- can use gen_random_uuid() without re-enabling it.
-- IF NOT EXISTS prevents an error if the extension is already installed.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- The players table stores all user accounts.
-- One row = one registered user.
CREATE TABLE players (
    -- id is the primary key — a UUID (Universally Unique Identifier).
    -- gen_random_uuid() generates a random UUID v4 like:
    --   550e8400-e29b-41d4-a716-446655440000
    -- We use UUIDs instead of auto-incrementing integers because:
    --   1. They are globally unique — safe to generate in multiple services
    --   2. They don't leak row counts to clients
    --   3. They make future database sharding easier
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- username is the player's display name shown on the leaderboard.
    -- UNIQUE ensures no two players share a username.
    -- NOT NULL means the column cannot be empty.
    -- VARCHAR(50) limits length to 50 characters.
    username VARCHAR(50) UNIQUE NOT NULL,

    -- email is used for login and account recovery.
    -- Also unique — each email maps to exactly one account.
    email VARCHAR(255) UNIQUE NOT NULL,

    -- password_hash stores the bcrypt hash of the player's password.
    -- We NEVER store plaintext passwords. bcrypt hashes are 60 chars long.
    password_hash VARCHAR(255) NOT NULL,

    -- role controls what actions the player is allowed to take.
    -- CHECK constraint acts as a database-enforced enum — PostgreSQL
    -- will reject any INSERT or UPDATE that sets role to a value not in this list.
    role VARCHAR(20) NOT NULL DEFAULT 'player'
        CHECK (role IN ('player', 'team_owner', 'moderator', 'admin')),

    -- profile_photo_url is the URL of the player's uploaded profile photo.
    -- Nullable: players don't need a photo to register.
    -- In Phase 1, uploads will be stored in an object store (e.g. S3 or MinIO).
    profile_photo_url VARCHAR(500),

    -- class_role is the player's chosen combat class in the arena game.
    -- Nullable: players choose their class after registration.
    -- Each class has different base stats (HP, armor) and available skills.
    class_role VARCHAR(20)
        CHECK (class_role IN ('tank', 'dps', 'healer', 'support')),

    -- skill_points_total is the player's total skill point budget.
    -- Players spend these points to allocate skills. Default is 20.
    -- The remaining points (total minus spent) are computed at read time —
    -- we don't store skill_points_remaining because it can get out of sync.
    skill_points_total INTEGER NOT NULL DEFAULT 20,

    -- kredits is the player's in-game currency balance.
    -- Players receive kredits from moderators or as transfers from other players.
    -- CHECK (kredits >= 0) prevents the balance from going negative.
    kredits INTEGER NOT NULL DEFAULT 0
        CHECK (kredits >= 0),

    -- created_at and updated_at are standard audit timestamps.
    -- TIMESTAMPTZ = TIMESTAMP WITH TIME ZONE — always stores UTC internally.
    -- Default NOW() sets the value automatically on INSERT.
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index on email: the login query looks up players by email.
-- Without an index, every login would do a full table scan.
-- WITH (fillfactor=90) leaves 10% of each page empty for future updates,
-- reducing page splits during heavy write loads.
CREATE INDEX idx_players_email ON players (email);

-- Index on username: used when looking up profiles by username.
CREATE INDEX idx_players_username ON players (username);
