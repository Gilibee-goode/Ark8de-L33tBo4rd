# Claude Code — Project Instructions for Ark8de-L33tBo4rd

These rules apply to ALL code written in this project, no exceptions.

---

## This is a Go Learning Project

The person working on this project is **learning Go for the first time**.
The goal is not just working code — it is code that teaches.
Correct code the learner cannot understand is a failure. Prioritise clarity over brevity.

---

## Rule 1: Orient Before You Build

Before starting any task, read **PROGRESS.md** and **ARCHITECTURE.md** to understand what exists, what's next, and which patterns are established. Do not assume — these files are the source of truth.

---

## Rule 2: Code Structure — 3-Layer Pattern

Every domain package (`auth`, `player`, `team`, `leaderboard`) follows:

| File | Layer | Allowed to touch |
|---|---|---|
| `handler.go` | HTTP | Parse request, call service, write response. No SQL, no game logic. |
| `service.go` | Business logic | Validation, rules, computation. No HTTP, no SQL. |
| `repository.go` | Database | SQL queries, return domain types. No HTTP, no game logic. |
| `model.go` | Types | Structs, constants, sentinel errors. |

Services depend on a `Repository` interface (defined in `service.go`) for mock injection in tests.
Add a comment at the top of each file stating which layer it is and what it may do.

---

## Rule 3: Comment Everything That Isn't Obvious

**Comment:** every function/method (what, receives, returns), every struct and field (what it represents in the game), any Go feature a beginner wouldn't know, any non-trivial conditional (why, not what), any SQL query (what it fetches and why), any error handling (what went wrong and why we handle it this way).

**Don't comment** the truly self-evident (`i++`). The bar: "would a developer new to Go understand this without a comment?"

**Explain imports** — group standard library / third-party / internal with a blank line and label. Annotate each package with what it is and why it's needed in this file.

**Explain Go concepts inline on first use** — not just what the line does, but what the concept is:
`defer`, interfaces, goroutines, channels, struct embedding, error wrapping (`%w`), pointer vs value receivers, type assertions, `init()`, blank identifier `_`.

### Example of good style:

```go
import (
    // --- Standard library ---
    "context" // provides Context for cancellation — passed into DB queries and HTTP handlers
    "fmt"     // string formatting and error wrapping with fmt.Errorf

    // --- Third-party ---
    "github.com/jackc/pgx/v5/pgxpool" // PostgreSQL connection pool — faster than database/sql for Postgres
)

// PlayerRepository handles all database operations for players.
// It is the only layer allowed to talk to the DB — business logic lives in PlayerService.
type PlayerRepository struct {
    db *pgxpool.Pool // connection pool to PostgreSQL
}

// GetByID fetches a player by UUID. Returns (Player, nil) on success or (nil, error) if not found.
func (r *PlayerRepository) GetByID(ctx context.Context, id uuid.UUID) (*Player, error) {
    // QueryRow returns at most one row. $1 is a placeholder (prevents SQL injection).
    row := r.db.QueryRow(ctx, "SELECT id, username, email, role FROM players WHERE id = $1", id)

    var p Player
    // Scan reads columns into struct fields. Returns pgx.ErrNoRows if the player doesn't exist.
    if err := row.Scan(&p.ID, &p.Username, &p.Email, &p.Role); err != nil {
        return nil, fmt.Errorf("GetByID: %w", err) // %w wraps the error so callers can unwrap it
    }
    return &p, nil
}
```

---

## Rule 4: Error Messages Must Be Human-Readable

Wrap errors with context at every layer so the full chain is visible in logs:

```go
// Good — says where, what, and why
return fmt.Errorf("PlayerService.AllocateSkills: player %s has %d SP remaining, need %d: %w",
    playerID, remaining, cost, ErrInsufficientSkillPoints)

// Bad
return err
```

---

## Rule 5: Every Change Gets Tests

- **Service-layer logic** → unit tests with hand-written mocks (see `internal/auth/service_test.go` for the pattern)
- **New endpoints or middleware** → integration tests via `httptest` (see `tests/` directory)
- **Bug fixes** → a test that reproduces the bug before the fix

Run the relevant suite before declaring done. If existing tests break, fix them in the same task.

```bash
cd monolith && go test ./internal/... -v   # unit (fast, no DB)
cd monolith && go test ./tests/... -v      # integration (needs Docker)
```

---

## Rule 6: Document as You Go

Each completed segment of work must include:
1. **PROGRESS.md updated** — tick the checkbox, add a note if scope changed
2. **Phase docs** — when a phase milestone is reached, create `docs/phase-N-complete.md` with:
   - What was built (brief summary)
   - Mermaid diagrams of information flow and DB schema
   - Table of new Go concepts introduced (one-line explanation each)
   - What the next phase will add
3. **Code-level docs** — per Rules 3–4 (comments on functions, imports, concepts)

Documentation ships with the code, not after it.

---

## Rule 7: Seed Data Must Reflect the Real Game

The seed script must create: at least 2 full teams of 4+ players, all 4 class roles represented, some players with gear and some without, one team gear pool positive and one negative, at least one moderator and one admin account, skills seeded for all class roles. Seed credentials documented at the top of the seed file.

---

## Codebase Map

```
monolith/
├── cmd/
│   ├── server/main.go        (173L)  HTTP server — env, DB connect, router, graceful shutdown
│   ├── migrate/main.go       (142L)  CLI for golang-migrate up/down
│   ├── seed/main.go          (573L)  Seeds 2 teams, 11 players, skills, gear, kredits
│   ├── auth-service/                  Phase 3 microservice :8081 — /auth/*
│   ├── player-service/                Phase 3 microservice :8082 — /api/players/*, /api/skills (+NATS)
│   ├── team-service/                  Phase 3 microservice :8083 — /api/teams/* (+NATS)
│   ├── leaderboard-service/           Phase 3 microservice :8084 — /api/leaderboard/* (NATS subscriber)
│   ├── frontend-service/              Phase 3 microservice :8085 — HTML pages, /static, sessions
│   └── gateway/                       Phase 3 api-gateway :8080 — reverse proxy + edge JWT validation
│
├── internal/
│   ├── app/router.go         (196L)  Builds chi router — shared by server and tests
│   ├── app/service_routers.go        Per-service router builders (Phase 3)
│   ├── app/run.go                    Shared service bootstrap (env, DB, NATS, graceful shutdown)
│   ├── events/                       NATS JetStream publisher/subscriber + event payload types
│   │
│   ├── auth/                          Register, login, JWT
│   │   ├── handler.go        (158L)  POST /auth/register, POST /auth/login, GET /auth/me
│   │   ├── service.go        (348L)  Password hashing, JWT generation, validation
│   │   ├── repository.go     (189L)  CreatePlayer, GetByEmail, GetByID
│   │   ├── model.go          (135L)  Player struct, requests, roles
│   │   └── service_test.go   (341L)  11 unit tests
│   │
│   ├── player/                        Class, skills, gear, kredits, stats
│   │   ├── handler.go        (254L)  /api/players/me/* endpoints
│   │   ├── service.go        (349L)  Stat computation, skill budget, kredit transfer
│   │   ├── repository.go     (530L)  17 methods — skills, gear, kredits, stats
│   │   ├── model.go          (156L)  Skill, GearType, PlayerStats, KreditTransaction
│   │   └── service_test.go   (603L)  22 unit tests
│   │
│   ├── team/                          Teams, join requests, arkade points
│   │   ├── handler.go        (298L)  /api/teams/* endpoints
│   │   ├── service.go        (409L)  Authorization, join flow, lock-in, points
│   │   ├── repository.go     (546L)  19 methods — CRUD, members, joins, points
│   │   ├── model.go          (90L)   Team, JoinRequest, ArkadePointLog
│   │   └── service_test.go   (797L)  33 unit tests
│   │
│   ├── leaderboard/                   Ranked team list
│   │   ├── handler.go        (50L)   GET /api/leaderboard, GET /api/leaderboard/teams/:id
│   │   ├── service.go        (47L)   Pure delegation
│   │   ├── repository.go     (67L)   RANK() window function query
│   │   └── model.go          (23L)   LeaderboardEntry
│   │
│   ├── frontend/handler.go   (282L)  Server-rendered HTML — leaderboard, team detail
│   ├── middleware/auth.go     (232L)  JWT extraction, role enforcement, context injection
│   ├── db/db.go              (65L)   pgxpool connection setup
│   └── respond/respond.go    (64L)   JSON/error response helpers
│
├── tests/                             Integration tests (55 tests, real PostgreSQL)
│   ├── setup_test.go         (456L)  TestMain — testcontainers or DATABASE_URL
│   ├── container_test.go     (134L)  testcontainers-go PostgreSQL helper
│   ├── auth_test.go          (218L)  11 subtests
│   ├── player_test.go        (422L)  16 subtests
│   ├── team_test.go          (393L)  23 subtests
│   ├── leaderboard_test.go   (110L)  3 subtests
│   └── frontend_test.go      (116L)  5 tests
│
├── templates/                         Go HTML templates (dark theme)
├── static/                            CSS, images
├── Dockerfile                         Multi-stage scratch build, parameterized: --build-arg SERVICE=<cmd>
└── go.mod

db/migrations/                         13 SQL migration pairs (schema source of truth)
infra/docker-compose.yml               Local dev: postgres + nats + migrate + 6 services + pgadmin
infra/terraform/                       Phase 4: modules/cluster (KIND) + environments/dev
infra/k8s/base/                        Phase 5: per-service Deployment/Service/ConfigMap/HPA, jobs, ingress, SealedSecret
infra/k8s/helm-values/                 postgres (Bitnami), nats, ingress-nginx values
docs/                                  Phase docs with Mermaid diagrams
```
