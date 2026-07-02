# Phase 3 Complete: Service Extraction

## What was built

The monolith was split into **six independently deployable services** plus NATS JetStream for async events. Extraction uses the **multi-binary monorepo** pattern: one Go module, one `internal/` codebase, six `cmd/` entry points — each service compiles to its own binary and container image and serves only its own routes. Physical code separation (separate modules per service) was deliberately skipped; the deployment units are already independent, and the shared module keeps the 130+ existing tests green.

| Service | Binary | Port | Serves | Events |
|---|---|---|---|---|
| auth-service | `cmd/auth-service` | 8081 | `/auth/*` | — |
| player-service | `cmd/player-service` | 8082 | `/api/players/*`, `/api/skills` | publishes `player.gear_updated` |
| team-service | `cmd/team-service` | 8083 | `/api/teams/*` | publishes `team.points_updated`, `team.membership_changed` |
| leaderboard-service | `cmd/leaderboard-service` | 8084 | `/api/leaderboard/*` | subscribes to `team.>` (durable consumer) |
| frontend-service | `cmd/frontend-service` | 8085 | HTML pages, `/static`, session login | — |
| api-gateway | `cmd/gateway` | 8080 | everything — reverse-proxies by prefix, rejects invalid JWTs at the edge | — |

New packages:
- `internal/events` — `Publisher` interface, JSON payload types, NATS JetStream publisher (stream `ARK8DE`, subjects `team.>`/`player.>`), `NoopPublisher` for monolith/test mode
- `internal/app/service_routers.go` — one `Build*Router` per service (same route patterns as the monolith)
- `internal/app/run.go` — shared bootstrap (env, logging, DB, NATS, graceful shutdown)

One parameterized Dockerfile builds every image: `docker build -f monolith/Dockerfile --build-arg SERVICE=team-service .` (context = repo root; `migrate`/`seed` images bake in `db/migrations`).

## Information flow

```mermaid
flowchart LR
    Browser --> GW[api-gateway :8080]
    GW -->|/auth/*| AUTH[auth-service]
    GW -->|/api/players/*| PLAYER[player-service]
    GW -->|/api/teams/*| TEAM[team-service]
    GW -->|/api/leaderboard/*| LB[leaderboard-service]
    GW -->|HTML, /static| FE[frontend-service]
    AUTH & PLAYER & TEAM & LB & FE --> PG[(PostgreSQL)]
    PLAYER --)|player.gear_updated| NATS[NATS JetStream]
    TEAM --)|team.points_updated<br/>team.membership_changed| NATS
    NATS --)|durable consumer| LB
```

## Design decisions & trade-offs

- **Shared module, separate binaries** — extraction stays mechanical; a service can be moved to its own module later without changing its API.
- **Frontend reads the DB directly** (like the other services) instead of calling them over REST — acceptable while all services share one database; revisit when data ownership splits.
- **Gateway validates JWT signatures** for any Bearer token and 401s garbage before it reaches a service; role enforcement stays inside services (defense in depth).
- **Events are best-effort** — publish failures are logged, never fail the request; the monolith and all unit tests run with the noop publisher (no NATS needed).
- **Monolith still works** — `cmd/server` is untouched, so the integration suite exercises the same routers the services use.

## Verified

- All unit tests (77) + integration tests (70) pass.
- `docker compose up --build`: postgres + nats + migrate + 6 services; gateway smoke-tested (`/healthz`, `/api/leaderboard`, `/api/teams/`, `/login` → 200; invalid JWT → 401).
- E2E: moderator login → `PUT /api/teams/:id/points` via gateway → `team.points_updated` received by leaderboard-service over NATS.

## Next phase

Phase 4 provisions a local KIND Kubernetes cluster with Terraform; Phase 5 deploys these images onto it.
