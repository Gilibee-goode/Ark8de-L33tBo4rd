# Ark8de-L33tBo4rd

A leaderboard for **The Ark8de** — a physical group-vs-group arena game. Players pick a class, allocate skills, equip gear, join teams, and compete for Arkade Points. This repo is also a structured learning path through Go and DevOps, built phase by phase.

---

## Current Status

| Phase | Description | Status |
|---|---|---|
| 0 | Foundation — Docker, Postgres, `/healthz` | ✅ Complete |
| 1 | Auth — register, login, JWT, role middleware | 🔨 In progress |
| 2 | Testing + CI | Not started |
| 3 | Microservices + NATS | Not started |
| 4 | Terraform + KIND | Not started |
| 5 | Kubernetes | Not started |
| 6 | GitOps with ArgoCD | Not started |
| 7 | Observability — Prometheus + Grafana | Not started |

**Live endpoints today:**
```
GET  /healthz          → 200 OK if server and DB are up
POST /auth/register    → create a player account
POST /auth/login       → get a JWT token
GET  /auth/me          → your player profile (requires JWT)
```

---

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) or [OrbStack](https://orbstack.dev/)
- [Go 1.23+](https://go.dev/dl/)
- [Task](https://taskfile.dev/) — `brew install go-task`

---

## Setup

**1. Clone and enter the repo**
```bash
git clone https://github.com/Gilibee-goode/ark8de-l33tbo4rd.git
cd ark8de-l33tbo4rd
```

**2. Create your local environment file**
```bash
cp .env.example monolith/.env
```
The defaults work for local dev — no edits needed.

**3. Start the stack**
```bash
task dev
```
This builds the Go server image and starts three containers: Postgres, pgAdmin, and the app.

**4. Run the database migrations** (in a second terminal)
```bash
task migrate-up
```
This creates all 11 tables in the database.

**5. Verify it's running**
```bash
curl http://localhost:8080/healthz
# → ok
```

---

## Tear Down

Stop all containers (data is preserved):
```bash
task down
```

Stop and wipe all data (full reset):
```bash
docker compose -f infra/docker-compose.yml down -v
```

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
task seed       # populate the DB with test data (Phase 1)
task deps       # tidy Go module dependencies
```

---

## Things to Try

### Check the server is up
```bash
curl http://localhost:8080/healthz
```

### Register a player account
```bash
curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username": "gili", "email": "gili@example.com", "password": "Secret123"}' \
  | jq .
```
You'll get back a JWT token and your player profile.

### Log in
```bash
curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "gili@example.com", "password": "Secret123"}' \
  | jq .
```

### Fetch your profile (paste your token from above)
```bash
curl -s http://localhost:8080/auth/me \
  -H "Authorization: Bearer <your_token_here>" \
  | jq .
```

### See what happens with bad input
```bash
# Weak password
curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username": "test", "email": "test@example.com", "password": "weak"}' \
  | jq .

# Duplicate email
curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username": "othername", "email": "gili@example.com", "password": "Secret123"}' \
  | jq .

# Missing or invalid token
curl -s http://localhost:8080/auth/me | jq .
```

### Browse the database
Open [http://localhost:5050](http://localhost:5050) in your browser (pgAdmin).
- Email: `admin@ark8de.dev`
- Password: `admin`

Connect to the server:
- Host: `localhost`
- Port: `5432`
- Database: `ark8de`
- Username: `ark8de`
- Password: `ark8de_dev`

Then navigate: **Servers → ark8de → Databases → ark8de → Schemas → public → Tables**

Right-click any table → **View/Edit Data → All Rows** to see the data as a spreadsheet.

---

## Project Structure

```
monolith/
  cmd/server/main.go         # entry point — starts HTTP server
  cmd/migrate/main.go        # migration CLI tool
  internal/
    auth/                    # register, login, JWT
      model.go               # Player struct, request/response types
      repository.go          # SQL queries (players table)
      service.go             # business logic, bcrypt, JWT signing
      handler.go             # HTTP handlers
    middleware/
      auth.go                # JWT verification + role enforcement
    respond/
      respond.go             # shared JSON response helpers
    db/
      db.go                  # database connection pool

db/migrations/               # SQL migration files (source of truth for schema)
infra/
  docker-compose.yml         # local dev stack
scripts/
  seed.go                    # test data (scaffold — implemented in Phase 1)
Taskfile.yml                 # developer commands
```
