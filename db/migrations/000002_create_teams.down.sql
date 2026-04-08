-- Migration: 000002_create_teams (DOWN)
-- Drops the teams table and all dependent objects.

DROP TABLE IF EXISTS teams CASCADE;
