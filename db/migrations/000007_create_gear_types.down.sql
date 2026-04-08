-- Migration: 000007_create_gear_types (DOWN)
-- Drops the gear_types table (and its seeded data).

DROP TABLE IF EXISTS gear_types CASCADE;
