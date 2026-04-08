-- Migration: 000001_create_players (DOWN)
-- Reverses the UP migration by dropping the players table and indexes.
-- The indexes are dropped automatically when the table is dropped.
-- Run with: go run ./cmd/migrate down

-- DROP TABLE removes the table and all its data permanently.
-- CASCADE also drops any objects that depend on this table
-- (foreign keys, views, etc.) — use with caution in production.
-- In development, CASCADE is safe because other tables will be
-- re-created by their own UP migrations.
DROP TABLE IF EXISTS players CASCADE;

-- Drop the pgcrypto extension if no other migration uses it.
-- We leave it in place here because later migrations also use gen_random_uuid().
-- If you drop players but not other tables, pgcrypto is still needed.
-- (Extensions are safe to leave; they don't affect data.)
