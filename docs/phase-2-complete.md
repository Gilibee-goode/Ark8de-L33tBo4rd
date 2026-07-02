# Phase 2 Complete — Testing, Structured Logging, Docker Optimization

## What Was Built

Phase 2 added a comprehensive test suite, structured logging, and a production-optimized Docker image to the monolith built in Phase 1.

| Deliverable | Details |
|---|---|
| **Integration tests** | 55 tests across 5 files — full HTTP stack with real PostgreSQL |
| **Unit tests** | 66 tests across 3 files — service-layer business logic with hand-written mocks |
| **Testcontainers** | Auto-launches disposable PostgreSQL for tests — zero manual setup |
| **Structured logging** | `log/slog` with JSON output across all handlers and services |
| **Docker optimization** | Scratch-based image: **13.3 MB** (down from 22 MB with Alpine) |
| **Shared router** | `internal/app/router.go` — same router in production and tests |
| **Repository interfaces** | Enables mock injection for unit testing without changing callers |

**Deferred:** GitHub Actions CI pipeline (will be added later).

---

## Architecture After Phase 2

```mermaid
flowchart TB
    subgraph "Test Suite (121 tests)"
        subgraph "Unit Tests (66 — no DB)"
            UT_Auth["auth/service_test.go<br/>11 tests"]
            UT_Player["player/service_test.go<br/>22 tests"]
            UT_Team["team/service_test.go<br/>33 tests"]
        end

        subgraph "Integration Tests (55 — real PostgreSQL)"
            IT_Auth["tests/auth_test.go<br/>11 subtests"]
            IT_Player["tests/player_test.go<br/>16 subtests"]
            IT_Team["tests/team_test.go<br/>23 subtests"]
            IT_LB["tests/leaderboard_test.go<br/>3 subtests"]
            IT_FE["tests/frontend_test.go<br/>5 tests"]
        end
    end

    subgraph "Application (monolith)"
        Router["chi Router<br/>(app.BuildRouter)"]
        Handlers["Handlers"]
        Services["Services"]
        Repos["Repositories"]
    end

    subgraph "Infrastructure"
        TC["testcontainers-go<br/>postgres:16-alpine"]
        DB[(PostgreSQL)]
        Docker["Docker Image<br/>scratch — 13.3 MB"]
    end

    UT_Auth & UT_Player & UT_Team -->|"mock repos"| Services
    IT_Auth & IT_Player & IT_Team & IT_LB & IT_FE -->|"HTTP requests"| Router
    Router --> Handlers --> Services --> Repos --> DB
    TC -->|"auto-launches"| DB
    Docker -->|"contains"| Router
```

---

## Database Schema (unchanged from Phase 1)

```mermaid
erDiagram
    players {
        uuid id PK
        varchar username UK
        varchar email UK
        varchar password_hash
        varchar role
        varchar class_role
        int skill_points_total
        int kredits
    }

    teams {
        uuid id PK
        varchar name UK
        varchar tag UK
        uuid owner_id FK
        int arkade_points
        int gear_points_total
        boolean is_locked_in
    }

    team_members {
        uuid team_id FK
        uuid player_id FK
    }

    join_requests {
        uuid id PK
        uuid team_id FK
        uuid player_id FK
        varchar status
    }

    skills {
        uuid id PK
        varchar name
        varchar class_role
        int cost_skill_points
        int hp_bonus
        int armor_bonus
    }

    player_skill_allocations {
        uuid player_id FK
        uuid skill_id FK
    }

    gear_types {
        uuid id PK
        varchar name
        int gear_point_cost
    }

    player_gear {
        uuid player_id FK
        uuid gear_type_id FK
    }

    kredit_transactions {
        uuid id PK
        uuid from_player_id FK
        uuid to_player_id FK
        int amount
        varchar note
    }

    arkade_point_logs {
        uuid id PK
        uuid team_id FK
        uuid changed_by FK
        int delta
        varchar reason
    }

    leaderboard_entries {
        uuid team_id FK
        int arkade_points
        int rank
    }

    players ||--o{ team_members : "belongs to"
    teams ||--o{ team_members : "has members"
    teams ||--o{ join_requests : "receives"
    players ||--o{ join_requests : "sends"
    players ||--o{ player_skill_allocations : "allocates"
    skills ||--o{ player_skill_allocations : "allocated to"
    players ||--o{ player_gear : "selects"
    gear_types ||--o{ player_gear : "selected by"
    players ||--o{ kredit_transactions : "sends/receives"
    teams ||--o{ arkade_point_logs : "earns"
    teams ||--o| leaderboard_entries : "ranked in"
```

---

## Data Flow: Running Tests

```mermaid
sequenceDiagram
    participant Dev as Developer
    participant Go as go test
    participant TC as testcontainers-go
    participant Docker as Docker Daemon
    participant PG as PostgreSQL Container
    participant App as httptest.Server

    Dev->>Go: go test ./tests/... -v
    Go->>TC: startPostgresContainer()
    TC->>Docker: Create postgres:16-alpine
    Docker->>PG: Start on random port
    TC->>PG: Wait for ready (log detection)
    TC->>PG: Run 12 migrations
    Go->>App: Start httptest.Server (app.BuildRouter)
    Go->>PG: Truncate data tables

    loop 55 integration tests
        Go->>App: HTTP request
        App->>PG: SQL query
        PG->>App: Result
        App->>Go: HTTP response
        Go->>Go: Assert status + body
    end

    Go->>App: Close server
    Go->>Docker: Terminate container
    Docker->>PG: Stop and remove
    Go->>Dev: PASS (55 tests, ~13s)
```

---

## Key Go Concepts Introduced in Phase 2

| Concept | Explanation |
|---|---|
| **`TestMain(m *testing.M)`** | Special entry point that runs before tests — used for one-time setup (DB, server) and teardown |
| **`httptest.NewServer`** | Creates a real HTTP server on a random port for integration testing |
| **Interfaces for testability** | Defining `Repository` interfaces lets tests inject mocks without changing production code |
| **Structural typing** | Go interfaces are satisfied implicitly — no `implements` keyword needed |
| **Function-field mocks** | A struct with `func` fields lets each test control what the mock returns |
| **`var _ Interface = (*Type)(nil)`** | Compile-time check that a concrete type implements an interface |
| **`errors.Is` / `errors.As`** | Check error chains for sentinel errors or extract specific error types |
| **testcontainers-go** | Library that creates real Docker containers from Go test code |
| **`wait.ForLog().WithOccurrence(2)`** | Wait strategy: container is ready when a log message appears N times |
| **Blank imports (`_ "pkg"`)** | Import solely for side effects (e.g. registering a database driver via `init()`) |
| **`log/slog`** | Go's standard structured logger (Go 1.21+) — outputs key-value pairs, not printf strings |
| **`slog.NewJSONHandler`** | Produces JSON log output, ideal for log aggregators (Loki, Datadog, ELK) |
| **`scratch` Docker image** | Empty base image (0 bytes) — only works with statically linked binaries |
| **`-ldflags="-w -s"`** | Strips debug info and symbol table from Go binary, reducing size ~30% |
| **`CGO_ENABLED=0`** | Disables C bindings, producing a fully static binary that runs on any Linux |

---

## Docker Image Optimization

| Technique | Saving |
|---|---|
| Multi-stage build (discard Go toolchain) | ~600 MB |
| `scratch` instead of `alpine:3.20` | ~8 MB |
| Binary stripping (`-ldflags="-w -s"`) | ~5 MB |
| `CGO_ENABLED=0` (static linking) | Enables scratch |
| CA certs copied from builder (not installed) | ~1.5 MB |
| **Final image size** | **13.3 MB** |

---

## What Phase 3 Will Add

Phase 3 is **Service Extraction** — breaking the monolith into independent microservices:

| Component | Description |
|---|---|
| leaderboard-service | First extraction — simplest service (2 endpoints, read-only) |
| auth-service | JWT issuance and validation |
| player-service | Class, skills, gear, kredits |
| team-service | Teams, join requests, Arkade points |
| NATS JetStream | Event bus for inter-service communication |
| api-gateway | Single entry point routing to all services |
