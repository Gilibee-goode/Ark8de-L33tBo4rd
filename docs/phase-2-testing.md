# Phase 2 — Integration Test Suite

## What Was Built

A comprehensive integration test suite that exercises every API endpoint and HTML page
through the full application stack: HTTP request -> chi router -> handler -> service -> repository -> PostgreSQL.

**55 test cases** across **5 test files**, all using Go's standard `testing` package
and `net/http/httptest` — no third-party test frameworks.

---

## Architecture Decision: Integration Tests, Not Unit Tests

The application's services accept concrete repository types, not interfaces:

```go
// This is what we have — concrete types, no interfaces
type PlayerService struct {
    repo *PlayerRepository  // concrete, not an interface
}
```

This means we **can't easily mock** repositories for unit testing. Instead of adding
interfaces purely for testability, we chose full-stack integration tests that hit a
real PostgreSQL database. This catches bugs that mocks would miss:

- SQL syntax errors and missing columns
- Transaction isolation issues
- Foreign key constraint violations
- Middleware misconfiguration (wrong route, missing auth)

---

## How It Works

```mermaid
flowchart LR
    subgraph "Test Process"
        TestMain["TestMain()"]
        Tests["Test Functions"]
    end

    subgraph "httptest Server"
        Router["chi Router<br/>(app.BuildRouter)"]
        MW["Middleware<br/>RequestID + Logger + Auth"]
        Handlers["Handlers<br/>auth + player + team + leaderboard + frontend"]
    end

    TestMain -->|"1. Connect to PostgreSQL"| DB[(PostgreSQL)]
    TestMain -->|"2. Build shared router"| Router
    TestMain -->|"3. Start httptest.NewServer"| Server["httptest.Server<br/>http://127.0.0.1:random"]
    TestMain -->|"4. Truncate data tables"| DB
    TestMain -->|"5. Run tests"| Tests

    Tests -->|"HTTP requests"| Server
    Server --> Router --> MW --> Handlers
    Handlers --> DB
```

### Key Design Choices

| Choice | Why |
|---|---|
| **Shared router** (`internal/app/router.go`) | Tests use the exact same router as production — no hand-assembled imitation that could drift |
| **Real PostgreSQL** (same as docker-compose) | No mocks, no SQLite — tests catch real DB issues |
| **`httptest.NewServer`** | Standard library; starts a real HTTP server on a random port, no Docker needed for the test server itself |
| **Truncate between runs** | `TestMain` clears data tables but preserves reference data (skills, gear_types) |
| **No third-party test libs** | Uses only `testing`, `net/http`, `encoding/json` — no testify, no gomega, nothing to learn beyond Go stdlib |

---

## Shared Router Extraction

The router was extracted from `cmd/server/main.go` into `internal/app/router.go`:

```go
// Before: all wiring was inline in main()
func main() {
    // ... 200 lines of repo → service → handler → route registration
}

// After: main.go calls the shared builder
r := app.BuildRouter(pool, jwtSecret, "templates")
```

Both `cmd/server/main.go` and `tests/setup_test.go` call `app.BuildRouter()` with their
own database pool and JWT secret. This guarantees tests exercise the real middleware chain.

---

## Test Files and Coverage

### `tests/setup_test.go` — Test Infrastructure

| Component | Purpose |
|---|---|
| `TestMain(m *testing.M)` | Connects to DB, builds router, starts httptest server, truncates tables, runs all tests |
| `truncateDataTables()` | Clears players, teams, etc. while preserving skills and gear_types |
| `doRequest(t, method, path, body, token)` | Makes HTTP requests with optional JSON body and JWT auth |
| `parseJSON(t, resp)` / `parseJSONArray(t, resp)` | Reads response body into `map[string]any` or `[]any` |
| `assertStatus(t, resp, code)` | Checks HTTP status code with helpful error message |
| `assertContains(t, haystack, needle)` | Checks string contains substring (for HTML pages) |
| `registerPlayer()` / `loginPlayer()` / `registerAndGetToken()` | Auth convenience helpers used across all test files |

### `tests/auth_test.go` — Authentication (11 subtests)

| Test | What it verifies |
|---|---|
| `TestAuthRegister/success` | 201 + token + player fields |
| `TestAuthRegister/duplicate_email` | 409 Conflict |
| `TestAuthRegister/duplicate_username` | 409 Conflict |
| `TestAuthRegister/weak_password` | 400 Bad Request |
| `TestAuthRegister/missing_fields` | 400 Bad Request |
| `TestAuthLogin/success` | 200 + token |
| `TestAuthLogin/wrong_password` | 401 Unauthorized |
| `TestAuthLogin/email_not_found` | 401 Unauthorized |
| `TestAuthMe/success` | 200 + correct username |
| `TestAuthMe/no_token` | 401 Unauthorized |
| `TestAuthMe/invalid_token` | 401 Unauthorized |

### `tests/player_test.go` — Player Endpoints (16 subtests)

| Test | What it verifies |
|---|---|
| `TestPlayerSetClass` | Set tank, switch to dps, invalid class, no auth |
| `TestPlayerGetStats` | HP/armor/SP returned for a player with a class |
| `TestPlayerSkills` | Allocate valid skills, check budget, wrong class rejection, invalid ID |
| `TestListSkills` | Empty without filter, correct results with `?class_role=tank` |
| `TestPlayerGear` | Select gear, verify points used, reject invalid gear ID |
| `TestPlayerKredits` | Initial balance 0, insufficient transfer, invalid amount, self-transfer rejected |
| `TestPlayerPublicProfile` | Correct fields, no email leaked, 404 for missing player |

### `tests/team_test.go` — Team Endpoints (23 subtests)

| Test | What it verifies |
|---|---|
| `TestTeamCreate` | Success (201), duplicate name (409), duplicate tag (409), invalid tag (400), no auth (401) |
| `TestTeamList` | Returns at least 1 team |
| `TestTeamLifecycle` | Full 16-step workflow: create -> join request -> accept -> detail -> Arkade points -> point history -> forbidden (non-mod) -> update -> lock -> unlock -> remove member -> cannot remove owner -> 404 |
| `TestJoinRequestReject` | Send request -> owner rejects -> member count unchanged |

### `tests/leaderboard_test.go` — Leaderboard (3 subtests)

| Test | What it verifies |
|---|---|
| `TestLeaderboard/full_leaderboard` | Two teams ranked correctly by Arkade points |
| `TestLeaderboard/team_card` | Single team's entry has correct name and points |
| `TestLeaderboard/not_found` | 404 for nonexistent team |

### `tests/frontend_test.go` — HTML Pages + Static Files (5 tests)

| Test | What it verifies |
|---|---|
| `TestFrontendLeaderboard` | GET `/` returns 200, contains "Ark8de" |
| `TestFrontendTeamDetail` | GET `/teams/{id}` renders team name and tag, 404 for missing team |
| `TestHealthz` | GET `/healthz` returns "ok" |
| `TestStaticCSS` | GET `/static/style.css` returns CSS with correct Content-Type |

---

## How to Run

```bash
# 1. Start the database (if not already running)
task dev

# 2. Apply migrations (if not already applied)
task migrate-up

# 3. Run the test suite
task test
# or directly:
cd monolith && go test ./tests/... -v
```

The tests connect to the same PostgreSQL instance as docker-compose dev
(`postgres://ark8de:ark8de_dev@localhost:5432/ark8de`). They truncate data
tables before running, so any seed data will be cleared.

To run specific test groups:

```bash
cd monolith
go test ./tests/... -v -run TestAuth        # only auth tests
go test ./tests/... -v -run TestTeamLifecycle # only the lifecycle test
go test ./tests/... -v -run TestPlayerSkills  # only skill allocation tests
```

---

## Key Go Testing Concepts

| Concept | Explanation |
|---|---|
| `TestMain(m *testing.M)` | Special function that runs instead of tests directly — gives you setup/teardown hooks around the entire test suite |
| `httptest.NewServer(handler)` | Creates a real HTTP server on a random port for testing — standard library, no Docker needed |
| `t.Run("name", func(t *testing.T) {...})` | Creates a subtest — subtests group related checks and show up as `TestParent/subtest_name` in output |
| `t.Helper()` | Marks a function as a test helper — when it fails, Go reports the caller's line number, not the helper's |
| `t.Fatalf(format, args...)` | Fails and stops the current test immediately — used when continuing would be meaningless |
| `map[string]any` | Go's generic JSON object — `any` (alias for `interface{}`) holds any type; JSON numbers decode as `float64` |
| `result["field"].(string)` | Type assertion — extracts a concrete type from an `any` value; panics if the type is wrong |
| `-count=1` flag | Disables test result caching — Go caches passing tests by default, which can mask issues |

---

## What's Next

The remaining Phase 2 items after the integration test suite:

| Item | Status |
|---|---|
| Integration tests with `httptest` | Done (this work) |
| Shared router extraction | Done (this work) |
| Unit tests with mock repositories | Not started |
| `testcontainers-go` for isolated DB tests | Not started |
| GitHub Actions CI pipeline | Not started |
| Structured `log/slog` throughout | Not started |
| Multi-stage Docker build < 20 MB | Already done (Phase 1 Dockerfile) |
