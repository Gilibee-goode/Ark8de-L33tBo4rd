# Ark8de-L33tBo4rd

A leaderboard for **The Ark8de** — a physical group-vs-group arena game. Players pick a class, allocate skills, equip gear, join teams, and compete for Arkade Points.

This repo doubles as a structured learning path through Go and DevOps, built phase by phase from a monolith to a fully observable Kubernetes deployment.

---

## Current Status

| Phase | Description | Status |
|---|---|---|
| 0 | Foundation — Docker, Postgres, migrations, `/healthz` | ✅ Complete |
| 1 | Monolith — auth, players, skills, gear, teams, kredits, leaderboard, frontend | ✅ Complete |
| 2 | Testing + CI — 121 tests, testcontainers, structured logging, 13 MB Docker image | ✅ Complete |
| 3 | Microservices + NATS | Not started |
| 4 | Terraform + KIND | Not started |
| 5 | Kubernetes | Not started |
| 6 | GitOps with ArgoCD | Not started |
| 7 | Observability — Prometheus + Grafana | Not started |

---

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) or [OrbStack](https://orbstack.dev/)
- [Go 1.25+](https://go.dev/dl/)
- [Task](https://taskfile.dev/) — `brew install go-task`

---

## Quick Start

```bash
# Clone
git clone https://github.com/Gilibee-goode/ark8de-l33tbo4rd.git
cd ark8de-l33tbo4rd

# Create local env file (defaults work out of the box)
cp .env.example monolith/.env

# Start Postgres + pgAdmin + app
task dev

# Apply database migrations (in a second terminal)
task migrate-up

# Seed with realistic test data (2 teams, 11 players, skills, gear, kredits)
task seed

# Verify
curl http://localhost:8080/healthz
# → ok
```

Open [http://localhost:8080](http://localhost:8080) in your browser to see the leaderboard.

---

## Things to Try

### Browse the frontend

| Page | URL | What you'll see |
|---|---|---|
| Leaderboard | [localhost:8080](http://localhost:8080) | Teams ranked by Arkade Points |
| Team detail | Click any team on the leaderboard | Roster, gear pool, Arkade point history |

### Play with the API

All API examples use `curl` and `jq`. Replace `<TOKEN>` with the JWT you get from register/login.

**Register and log in:**

```bash
# Register — you get back a JWT token and player profile
curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username": "gili", "email": "gili@ark8de.dev", "password": "Secret123"}' \
  | jq .

# Save your token for later commands
TOKEN=$(curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "gili@ark8de.dev", "password": "Secret123"}' \
  | jq -r .token)

echo $TOKEN
```

**Pick a class and view your stats:**

```bash
# Set your class to tank (also: dps, healer, support)
curl -s -X PUT http://localhost:8080/api/players/me/class \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"class_role": "tank"}' | jq .

# View your combat stats (HP, armor, skill points)
curl -s http://localhost:8080/api/players/me/stats \
  -H "Authorization: Bearer $TOKEN" | jq .
```

**Allocate skills:**

```bash
# See what skills are available for your class
curl -s "http://localhost:8080/api/skills?class_role=tank" | jq .

# Pick some skills (use the IDs from the response above)
curl -s -X PUT http://localhost:8080/api/players/me/skills \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"skill_ids": ["<skill-id-1>", "<skill-id-2>"]}' | jq .

# Check your stats again — HP and armor should have changed
curl -s http://localhost:8080/api/players/me/stats \
  -H "Authorization: Bearer $TOKEN" | jq .
```

**Equip gear:**

```bash
# View your current gear and the team gear pool
curl -s http://localhost:8080/api/players/me/gear \
  -H "Authorization: Bearer $TOKEN" | jq .

# Select gear (IDs are seeded by migrations — check the seed data)
curl -s -X PUT http://localhost:8080/api/players/me/gear \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"gear_type_ids": ["<gear-id-1>", "<gear-id-2>"]}' | jq .
```

**Create a team and invite players:**

```bash
# Create a team
curl -s -X POST http://localhost:8080/api/teams \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "The Legends", "tag": "LEG"}' | jq .

# View the leaderboard
curl -s http://localhost:8080/api/leaderboard | jq .

# View your team's detail (use the team ID from create response)
curl -s http://localhost:8080/api/teams/<team-id> | jq .
```

**See what happens with bad input:**

```bash
# Weak password (under 8 chars)
curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username": "test", "email": "t@t.com", "password": "weak"}' | jq .

# Invalid class
curl -s -X PUT http://localhost:8080/api/players/me/class \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"class_role": "wizard"}' | jq .

# No auth token
curl -s http://localhost:8080/auth/me | jq .

# Team tag with special characters
curl -s -X POST http://localhost:8080/api/teams \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "Bad Tag Team", "tag": "NO@WAY"}' | jq .
```

### Use the seed data

After running `task seed`, the database has pre-made accounts you can log into:

```bash
# Log in as the admin
TOKEN=$(curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "admin@ark8de.dev", "password": "Admin123!"}' \
  | jq -r .token)

# Grant Kredits to a player (moderator/admin only)
curl -s -X POST http://localhost:8080/api/players/<player-id>/kredits \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"amount": 500, "note": "Tournament prize"}' | jq .

# Add Arkade Points to a team (moderator/admin only)
curl -s -X PUT http://localhost:8080/api/teams/<team-id>/points \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"delta": 100, "reason": "Won round 1"}' | jq .
```

### Browse the database

Open [http://localhost:5050](http://localhost:5050) (pgAdmin).
- Email: `admin@ark8de.dev` / Password: `admin`

Connect to the server: Host `postgres`, Port `5432`, DB `ark8de`, User `ark8de`, Password `ark8de_dev`.

Navigate: **Servers → ark8de → Databases → ark8de → Schemas → public → Tables** — right-click any table → **View/Edit Data → All Rows**.

---

## Running Tests

Tests require Docker but **no manual database setup** — testcontainers handles everything.

```bash
cd monolith

# Unit tests — 66 tests, no DB needed (~2 seconds)
go test ./internal/... -v

# Integration tests — 55 tests, launches disposable PostgreSQL (~13 seconds)
go test ./tests/... -v

# Everything
go test ./... -v
```

See [docs/running-tests.md](docs/running-tests.md) for detailed options, flags, and troubleshooting.

---

## API Reference

### Public (no auth)

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/healthz` | Health check — returns `ok` if server and DB are up |
| `GET` | `/` | Leaderboard page (HTML) |
| `GET` | `/teams/{id}` | Team detail page (HTML) |
| `POST` | `/auth/register` | Create account — returns JWT + player profile |
| `POST` | `/auth/login` | Log in — returns JWT + player profile |
| `GET` | `/api/leaderboard` | All teams ranked by Arkade Points |
| `GET` | `/api/leaderboard/teams/{id}` | Single team's leaderboard card |
| `GET` | `/api/teams` | List all teams |
| `GET` | `/api/teams/{id}` | Team detail with roster and gear pool |
| `GET` | `/api/players/{id}` | Public player profile |
| `GET` | `/api/skills?class_role=tank` | List skills for a class |

### Authenticated (requires `Authorization: Bearer <token>`)

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/auth/me` | Your player profile |
| `PUT` | `/api/players/me/class` | Set your class (tank/dps/healer/support) |
| `GET` | `/api/players/me/stats` | Your computed combat stats |
| `GET` | `/api/players/me/skills` | Your allocated skills |
| `PUT` | `/api/players/me/skills` | Replace your skill allocation |
| `GET` | `/api/players/me/gear` | Your gear + team gear pool |
| `PUT` | `/api/players/me/gear` | Replace your gear selection |
| `GET` | `/api/players/me/kredits` | Your Kredit balance + history |
| `POST` | `/api/players/me/kredits/transfer` | Transfer Kredits to another player |
| `POST` | `/api/teams` | Create a team |
| `PUT` | `/api/teams/{id}` | Update team name/tag (owner or admin) |
| `DELETE` | `/api/teams/{id}` | Delete a team (owner or admin) |
| `PUT` | `/api/teams/{id}/lock` | Toggle team lock-in (owner or admin) |
| `DELETE` | `/api/teams/{id}/members/{pid}` | Remove a member (owner or admin) |
| `POST` | `/api/teams/{id}/join-requests` | Request to join a team |
| `GET` | `/api/teams/{id}/join-requests` | View pending join requests (owner) |
| `PUT` | `/api/teams/{id}/join-requests/{rid}` | Accept or reject a request (owner) |

### Moderator / Admin only

| Method | Endpoint | Description |
|---|---|---|
| `PUT` | `/api/teams/{id}/points` | Award or deduct Arkade Points |
| `GET` | `/api/teams/{id}/points/history` | Arkade Point audit log |
| `POST` | `/api/players/{id}/kredits` | Grant Kredits to a player |

---

## Available Commands

```bash
task            # list all commands
task dev        # start the full stack (postgres + pgadmin + app)
task down       # stop all containers
task migrate-up # apply all pending DB migrations
task migrate-down # roll back the last migration
task build      # compile the server binary locally
task test       # run all Go tests
task seed       # populate the DB with realistic test data
task deps       # tidy Go module dependencies
```

---

## Tear Down

```bash
# Stop containers (data preserved)
task down

# Full reset — stop containers and delete all data
docker compose -f infra/docker-compose.yml down -v
```

---

## Project Structure

```
monolith/
  cmd/
    server/main.go              # HTTP server entry point
    migrate/main.go             # database migration CLI
    seed/main.go                # test data seeder
  internal/
    app/router.go               # shared router (production + tests)
    auth/                       # register, login, JWT
    player/                     # class, skills, gear, kredits
    team/                       # teams, join requests, Arkade points
    leaderboard/                # ranked leaderboard
    frontend/                   # server-rendered HTML pages
    middleware/auth.go          # JWT verification + role enforcement
    respond/respond.go          # shared JSON response helpers
    db/db.go                    # database connection pool
  tests/                        # integration tests (testcontainers)
  templates/                    # Go HTML templates
  static/                       # CSS, images

db/migrations/                  # 12 SQL migration pairs (schema source of truth)
infra/docker-compose.yml        # local dev stack (Postgres + pgAdmin + app)
docs/                           # phase documentation with Mermaid diagrams
```

---

## Documentation

| Document | What it covers |
|---|---|
| [PROGRESS.md](PROGRESS.md) | Checkbox tracker for all 7 phases |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Domain model, API design, phased plan, infra layers |
| [docs/running-tests.md](docs/running-tests.md) | How to run unit, integration, and all tests |
| [docs/phase-0-complete.md](docs/phase-0-complete.md) | Foundation — Docker, migrations, healthz |
| [docs/phase-1-complete.md](docs/phase-1-complete.md) | Monolith — all features, flow diagrams |
| [docs/phase-2-complete.md](docs/phase-2-complete.md) | Testing + CI — test architecture, Docker optimization |
| [docs/phase-2-testing.md](docs/phase-2-testing.md) | Deep dive — test inventories, mock patterns, testcontainers |
