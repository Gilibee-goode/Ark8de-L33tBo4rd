# Running Tests

This project has three types of tests. Each type catches different bugs and has different prerequisites.

---

## Quick Reference

| Type | What it tests | Prerequisites | Speed | Command |
|---|---|---|---|---|
| **Unit tests** | Business logic (validation, auth, stat computation) | None | ~2 seconds | `cd monolith && go test ./internal/... -v` |
| **Integration tests** | Full HTTP stack (router → handler → service → repo → DB) | Docker running | ~13 seconds | `cd monolith && go test ./tests/... -v` |
| **All tests** | Everything above | Docker running | ~15 seconds | `cd monolith && go test ./... -v` |

---

## Unit Tests

Unit tests verify service-layer business logic in isolation — no database, no HTTP, no Docker. They use hand-written mock repositories to simulate database behavior.

**What they cover:**
- Input validation (weak passwords, invalid class roles, bad team tags)
- Authorization checks (owner vs admin vs random player)
- Error mapping (PostgreSQL error codes → user-facing errors)
- Stat computation (HP/armor from class + skills, skill point budgets)
- Edge cases (self-transfer, zero amounts, race condition guards)

**Run all unit tests:**

```bash
cd monolith
go test ./internal/... -v
```

**Run one package at a time:**

```bash
cd monolith
go test ./internal/auth/... -v      # 11 tests — register, login
go test ./internal/player/... -v    # 22 tests — class, skills, gear, kredits
go test ./internal/team/... -v      # 33 tests — teams, join requests, points
```

**Run a specific test by name:**

```bash
cd monolith
go test ./internal/player/... -v -run TestSetSkills_BudgetExceeded
go test ./internal/team/... -v -run TestResolveJoinRequest
```

The `-run` flag accepts a regex, so `TestResolveJoinRequest` matches all tests whose name starts with that prefix.

---

## Integration Tests (with testcontainers)

Integration tests exercise the full application stack by sending real HTTP requests to a real PostgreSQL database. By default, they use **testcontainers** to automatically launch a disposable PostgreSQL container — no manual setup needed.

**What they cover:**
- Every API endpoint (auth, player, team, leaderboard)
- HTML pages and static files (frontend, healthz, CSS)
- Middleware (JWT authentication, role-based access)
- SQL queries (joins, constraints, cascading deletes)
- Full multi-step workflows (create team → join → accept → add points → remove member)

**Prerequisites:** Docker must be running.

**Run all integration tests:**

```bash
cd monolith
go test ./tests/... -v
```

That's it. Testcontainers handles everything:
1. Pulls `postgres:16-alpine` (cached after first run)
2. Starts a container on a random port
3. Runs all 12 database migrations
4. Executes 55 test cases
5. Stops and removes the container

**Run a specific test group:**

```bash
cd monolith
go test ./tests/... -v -run TestAuth           # auth register/login/me
go test ./tests/... -v -run TestPlayerSkills    # skill allocation
go test ./tests/... -v -run TestTeamLifecycle   # full team workflow (16 steps)
go test ./tests/... -v -run TestLeaderboard     # ranked leaderboard
go test ./tests/... -v -run TestFrontend        # HTML pages
```

---

## Integration Tests (with external database)

If you already have PostgreSQL running (via `task dev`), you can skip testcontainers by setting `DATABASE_URL`. This is slightly faster because it skips container startup.

**Prerequisites:** PostgreSQL running with migrations applied.

```bash
# Start the database and apply migrations (one-time setup)
task dev
task migrate-up

# Run tests against the existing database
cd monolith
DATABASE_URL="postgres://ark8de:ark8de_dev@localhost:5432/ark8de?sslmode=disable" \
  go test ./tests/... -v
```

**When to use this mode:**
- You already have `task dev` running and want faster iteration
- You're debugging a test and want to inspect the database state afterward
- Testcontainers isn't working (Docker issues, CI environment limitations)

**Warning:** Tests truncate all data tables (players, teams, etc.) before running. Seed data from `task seed` will be cleared. Reference data (skills, gear_types) is preserved.

---

## Running All Tests Together

```bash
cd monolith

# Unit + integration (testcontainers)
go test ./... -v

# Unit + integration (external database)
DATABASE_URL="postgres://ark8de:ark8de_dev@localhost:5432/ark8de?sslmode=disable" \
  go test ./... -v
```

---

## Useful Flags

| Flag | What it does | Example |
|---|---|---|
| `-v` | Verbose — shows each test name and PASS/FAIL | `go test ./... -v` |
| `-run <regex>` | Run only tests matching the pattern | `-run TestLogin` |
| `-count=1` | Disable test caching (Go caches passing tests by default) | `-count=1` |
| `-timeout 60s` | Override the default 10-minute timeout | `-timeout 120s` |
| `-short` | Skip long-running tests (if any use `testing.Short()`) | `-short` |

---

## Troubleshooting

**"cannot start test database container"**
- Make sure Docker is running (`docker ps` should work)
- Or set `DATABASE_URL` to use an external database instead

**"FATAL: database ping failed"**
- If using `DATABASE_URL`: check the connection string and that PostgreSQL is running
- If using testcontainers: check Docker logs (`docker logs <container-id>`)

**"expected status 200, got 500"**
- An integration test is hitting a real bug. Check the test output — the response body is printed with the error. The server logs (printed to stdout during tests) often show the root cause.

**Tests are slow**
- First run with testcontainers pulls the Docker image (~30 seconds). Subsequent runs reuse the cached image (~5 seconds for container startup).
- Use `-run` to run only the tests you're working on during development.
- Unit tests are always fast (~2 seconds) — run those first while iterating.
