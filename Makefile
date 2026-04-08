# -----------------------------------------------------------------
# Makefile — Top-level developer commands for Ark8de-L33tBo4rd
# -----------------------------------------------------------------
# Run `make help` to see all available targets.
#
# HOW THIS WORKS:
#   Each target is a named rule. When you run `make <target>`, Make
#   executes the shell commands indented under that target name.
#
#   The ## comments after each target are parsed by the `help` target
#   to produce a self-documenting command list.
#
# CONVENTION:
#   .PHONY declares targets that are NOT files on disk — this tells
#   Make to always run them even if a file with that name exists.
# -----------------------------------------------------------------

.PHONY: help dev down migrate-up migrate-down test build seed deps

# Default target — running bare `make` prints the help menu.
.DEFAULT_GOAL := help

## help: Print this help message listing all available targets
help:
	@echo "Ark8de-L33tBo4rd — Available Make targets:"
	@echo ""
	# Parse this Makefile for lines matching `## target: description`
	# and format them into a clean two-column list.
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'
	@echo ""

# DOCKER_COMPOSE detects whether to use the v2 plugin (docker compose)
# or the standalone binary (docker-compose). Both are v2 — the syntax is identical.
# `docker compose version` exits 0 only if the compose plugin is properly installed.
DOCKER_COMPOSE := $(shell docker compose version > /dev/null 2>&1 && echo "docker compose" || echo "docker-compose")

## dev: Start all services (postgres, pgadmin, app) with Docker Compose — rebuilds the app image
dev:
	$(DOCKER_COMPOSE) -f infra/docker-compose.yml up --build

## down: Stop all running Docker Compose services and remove containers
down:
	$(DOCKER_COMPOSE) -f infra/docker-compose.yml down

## migrate-up: Run all pending database migrations (reads DATABASE_URL from env or .env)
migrate-up:
	# Load .env if it exists, then run the migrate tool from the monolith directory.
	# The migrate tool reads DATABASE_URL from the environment.
	cd monolith && go run ./cmd/migrate up

## migrate-down: Roll back the most recent database migration
migrate-down:
	cd monolith && go run ./cmd/migrate down

## test: Run all Go tests in the monolith with verbose output
test:
	cd monolith && go test ./... -v

## build: Compile the monolith server binary to monolith/bin/server
build:
	cd monolith && go build -o bin/server ./cmd/server

## seed: Run the seed script to populate the database with test data
seed:
	# The seed script lives outside the monolith module in scripts/,
	# so we reference it with a relative path from the monolith directory.
	cd monolith && go run ../scripts/seed.go

## deps: Tidy Go module dependencies (add missing, remove unused)
deps:
	cd monolith && go mod tidy
