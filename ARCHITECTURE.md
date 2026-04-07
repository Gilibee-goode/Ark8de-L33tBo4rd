# Ark8de-L33tBo4rd: Architecture & Implementation Plan

## Context
This is a greenfield personal project serving two purposes: a functional leaderboard for a physical group-vs-group arena game ("The Ark8de"), and a structured learning path through modern DevOps and Go development. The repo currently has only a README. We start from scratch.

**Learning-first project.** All code written in this project follows the rules in `CLAUDE.md`:
- Every function, struct, and non-obvious line is commented
- Every import is annotated explaining what the package is and why it's used
- Go concepts are explained inline the first time they appear
- After each phase, a `docs/phase-N-complete.md` is created with Mermaid flow diagrams of the app's information flow and DB schema at that point

---

## Domain Model

### User Roles (4 tiers)

| Role | Can Do |
|---|---|
| **Player** | Manage own class, skills, gear, kredits, profile photo; send team join requests |
| **Team Owner** | Everything a Player can do + create/delete team, upload logo, accept/remove members, lock team |
| **Moderator** | Everything a Team Owner can do + edit any player's stats, grant/subtract Kredits, assign Arkade points to teams |
| **Admin** | Unrestricted access — everything a Moderator can do + infrastructure-level operations |

A player becomes a Team Owner the moment they create a team. The role is stored on the `players` table and promoted/demoted by Admins.

---

### Core Entities

| Entity | Key Fields |
|---|---|
| **Player** | id, username, email, password_hash, role (player/team_owner/moderator/admin), profile_photo_url, class_role (tank/dps/healer/support), skill_points_total, kredits, created_at |
| **Team** | id, name, tag, owner_id → Player, logo_url, arkade_points, gear_points_total (default 12), is_locked_in, created_at |
| **TeamMember** | team_id → Team, player_id → Player, joined_at |
| **JoinRequest** | id, team_id → Team, player_id → Player, status (pending/accepted/rejected), requested_at, resolved_at |
| **Skill** | id, name, description, class_role, cost_skill_points, effect_description, effect_type (passive/active) |
| **PlayerSkillAllocation** | player_id → Player, skill_id → Skill, allocated_at |
| **GearType** | id, name (sword/shield/bow_and_arrow/spear), gear_point_cost |
| **PlayerGear** | player_id → Player, gear_type_id → GearType, selected_at |
| **KreditTransaction** | id, from_player_id → Player (nullable = moderator grant), to_player_id → Player, amount, note, created_by → Player, created_at |
| **ArkadePointLog** | id, team_id → Team, changed_by → Player, delta, reason, created_at |
| **LeaderboardEntry** | team_id, team_name (cached), arkade_points, rank, member_count, last_updated_at |

### Computed Player Stats (derived at read time, not stored)

| Stat | How it's computed |
|---|---|
| `skill_points_remaining` | `skill_points_total` − sum of allocated skill costs |
| `gear_points_used` | sum of gear_point_cost for all selected PlayerGear |
| `health_points` | base value from `class_role` + bonuses from allocated skills |
| `armor_points` | base value from `class_role` + bonuses from allocated skills |

### Gear Reference (seeded static data)

| Gear | Gear Point Cost |
|---|---|
| Sword | 2 |
| Bow & Arrow | 3 |
| Spear | 4 |
| Shield | 5 |

A team starts with **12 gear points**. Members draw from this shared pool. The pool can go negative if members overspend — this is visible to the team and flagged on the UI.

---

## Target Microservices Architecture

| Service | Owns | Responsibilities |
|---|---|---|
| **auth-service** | players, refresh_tokens | Login, JWT issuance, registration, profile photo upload |
| **player-service** | skills, player_skill_allocations, gear_types, player_gear, kredit_transactions | Class/role management, skill allocation, gear selection, computed stats, Kredit transfers |
| **team-service** | teams, team_members, join_requests, arkade_point_logs | Team CRUD, join requests, lock-in, logo upload, Arkade point assignment |
| **leaderboard-service** | leaderboard_entries | Read-optimised rankings by Arkade points, team roster display |
| **frontend-service** | none | Go templates + HTMX, SSR web UI |
| **api-gateway** | none | Single ingress, routing, JWT validation, role-based access enforcement |

**Service communication**: Synchronous REST for queries; async NATS JetStream events for state changes:
- `team.points_updated` → leaderboard-service recalculates rankings
- `player.gear_updated` → team-service recalculates team gear_points_used
- `team.membership_changed` → leaderboard-service updates cached member_count

---

## Key API Endpoints

### Auth
```
POST /auth/register                  - create account
POST /auth/login                     - returns JWT
GET  /auth/me                        - current player info
PUT  /auth/me/password
PUT  /auth/me/photo                  - upload profile photo (multipart)
```

### Players
```
GET  /players/:id                    - public profile
PUT  /players/me/class               - set class_role (tank/dps/healer/support)
GET  /players/me/stats               - computed HP, armor, skill points remaining, gear points used
GET  /players/me/skills              - allocated skills + points remaining
PUT  /players/me/skills              - update skill allocation (full replace)
GET  /players/me/gear                - selected gear + team gear pool impact
PUT  /players/me/gear                - update gear selection
GET  /players/me/kredits             - balance + transaction history
POST /players/:id/kredits            - [moderator+] grant Kredits to a player
POST /players/me/kredits/transfer    - transfer Kredits to another player or moderator
GET  /skills?class_role=tank         - list available skills for a role (public)
```

### Teams
```
GET    /teams                        - list all teams (public)
POST   /teams                        - create team (any player → becomes team_owner)
GET    /teams/:id                    - team detail + roster + gear pool status
PUT    /teams/:id                    - update name/tag [team_owner+]
DELETE /teams/:id                    - delete team [team_owner or admin]
PUT    /teams/:id/logo               - upload logo [team_owner+] (multipart)
PUT    /teams/:id/lock               - toggle lock-in [team_owner+]
DELETE /teams/:id/members/:pid       - remove member [team_owner or admin]
GET    /teams/:id/join-requests      - list pending requests [team_owner+]
POST   /teams/:id/join-requests      - send join request [player]
PUT    /teams/:id/join-requests/:rid - accept or reject [team_owner+]
PUT    /teams/:id/points             - add/subtract Arkade points [moderator+]
GET    /teams/:id/points/history     - Arkade point log [moderator+]
```

### Leaderboard
```
GET /leaderboard                     - all teams ranked by Arkade points (public)
GET /leaderboard/teams/:id           - single team card
```

---

## Phased Implementation Plan

### Phase 0: Foundation (Weeks 1-2)
**Goal**: Running dev environment with DB.

- Init Go module: `go mod init github.com/Gilibee-goode/ark8de-l33tbo4rd`
- Create repo structure (see Directory Structure below)
- `docker-compose.yml` with postgres + Go app + pgAdmin
- SQL migrations with `golang-migrate`
- `Makefile` targets: `dev`, `migrate-up`, `migrate-down`, `test`
- Seed script with sample data
- **Deliverable**: `docker compose up` -> Postgres with all tables + Go server at `GET /healthz`

### Phase 1: Monolith (Weeks 3-8)
**Goal**: Full working application as a single Go binary.

Internal packages mirror future service boundaries:
```
monolith/internal/
  auth/       -> handler.go, service.go, repository.go
  player/     -> class, skills, gear, stats, kredits
  team/       -> team CRUD, join requests, lock-in, arkade points
  leaderboard/
  middleware/ -> JWT validation, role checks (requireRole helper)
  uploads/    -> shared file upload handling (photos, logos)
  db/
```

Build order:
1. **Auth** — register, login, JWT, 4-tier role middleware — everything else needs this
2. **Player profile** — class_role selection, profile photo upload, computed stats endpoint
3. **Skills system** — seed skill data per role, allocation UI with point budget enforcement
4. **Gear system** — gear selection, team gear pool live display (can go negative — highlight in red)
5. **Team management** — create team, join requests, accept/reject, lock-in toggle, logo upload
6. **Kredits** — moderator grant, player-to-player transfer, transaction history
7. **Arkade points** — moderator assigns points to teams, audit log
8. **Leaderboard** — ranked team list by Arkade points, team roster cards
9. **Frontend** — Go templates + HTMX: leaderboard page, team detail page, player profile page, moderator panel

**Go libraries**:
- Router: `chi`
- DB: `pgx/v5` + `sqlc` (type-safe SQL codegen, no ORM)
- JWT: `golang-jwt/jwt/v5`
- Passwords: `golang.org/x/crypto/bcrypt`
- Config: `joho/godotenv`
- Migrations: `golang-migrate/migrate`
- Templates: stdlib `html/template`

**Deliverable**: Full app in Docker, accessible in browser. All CRUD works.

### Phase 2: Testing + CI (Weeks 9-10)
**Goal**: Establish test coverage and CI baseline before architectural complexity grows.

- Unit tests for all service layer functions (use interfaces + mock repositories)
- Integration tests with `httptest.NewRecorder`
- DB tests with `testcontainers-go` (real Postgres in Docker)
- GitHub Actions: `go vet`, `go test ./...`, `golangci-lint`
- Structured logging with `log/slog` (Go 1.21+ stdlib, no library needed)
- Multi-stage Docker build (target: image < 20MB, non-root user)

**Deliverable**: Green CI on every PR.

### Phase 3: Service Extraction (Weeks 11-16)
**Goal**: Split monolith into independent microservices.

Extraction order (least to most coupled):
1. **leaderboard-service** — lowest coupling, only reads; subscribes to events
2. **auth-service** — isolated, no runtime dependencies on other services
3. **player-service** — depends on auth; introduce NATS here for `player.gear_updated` events
4. **team-service** — depends on auth + player; publishes `team.points_updated` + `team.membership_changed`
5. **api-gateway** — add last; JWT validation and role enforcement move here

Update `docker-compose.yml` to run all services + NATS.

**Deliverable**: Each service independently deployable. One service failing degrades only that feature.

### Phase 4: Infrastructure as Code with Terraform + KIND (Weeks 17-20)
**Goal**: Provision all infrastructure reproducibly with code. Never manually create or destroy a cluster.

**What Terraform manages (dev):**
- A local KIND (Kubernetes IN Docker) cluster — free, instant, no cloud account needed
- KIND runs a full k8s cluster as Docker containers on your machine

**What Terraform manages (future prod):**
- Swap the `cluster` module backend for a cloud provider (Hetzner, DigitalOcean, AWS)
- Add: VPC / networking, DNS records, firewall rules
- The rest of the Terraform code stays unchanged — only the module implementation swaps

**Why KIND for learning:**
- Free (runs in Docker, no cloud cost)
- Cluster created in ~30 seconds
- Identical Terraform workflow to cloud — skills transfer 1:1
- `kind` Terraform provider (`tehcyx/kind`) manages it declaratively

**Example Terraform (dev/main.tf):**
```hcl
terraform {
  required_providers {
    kind = { source = "tehcyx/kind" }
  }
}

module "cluster" {
  source     = "../../modules/cluster"
  name       = "ark8de-dev"
  node_image = "kindest/node:v1.31.0"
}
```

**State backend:** Use Terraform Cloud (free tier) — handles remote state, locking, and plan history. Never commit `.tfstate` to git.

**Terraform directory structure:**
```
infra/terraform/
  environments/
    dev/
      main.tf          # uses kind module
      variables.tf
      outputs.tf       # cluster name, kubeconfig path
      terraform.tfvars # gitignored
    prod/              # future: uses cloud provider module
      main.tf
      variables.tf
      outputs.tf
  modules/
    cluster/           # provider-agnostic interface; dev=KIND, prod=cloud
    networking/        # future: VPC, subnets, firewall (cloud only)
    dns/               # future: A records -> load balancer IP (cloud only)
```

**Local workflow:**
```bash
terraform init                                  # download providers
terraform plan                                  # preview
terraform apply                                 # provision cluster
kubectl cluster-info --context kind-ark8de-dev  # verify
```

**GitHub Actions integration** (new `infra.yml` workflow):
- Triggers on PRs touching `infra/terraform/`
- `terraform fmt --check` + `terraform validate` in CI
- `terraform plan` output posted as a PR comment
- `terraform apply` runs on merge to `main` (for dev env; prod is manual/gated)

**Separation of concerns:**
- Terraform owns: "does the cluster exist?"
- ArgoCD owns: "what runs on it?"
- Never use Terraform to manage k8s Deployments/Services — that's GitOps territory

**Deliverable**: `terraform apply` in `environments/dev/` provisions a local KIND cluster. `kubectl get nodes` shows cluster nodes Ready.

### Phase 5: Kubernetes (Weeks 21-26)
**Goal**: Deploy on the Terraform-provisioned cluster. Learn k8s fundamentals through real workloads.

Per service: `Deployment`, `Service (ClusterIP)`, `ConfigMap`, `Secret`, `HPA`

Shared infra:
- PostgreSQL via Bitnami Helm chart (one instance, multiple databases/schemas)
- NATS via NATS Helm chart
- `ingress-nginx` for external traffic -> api-gateway

Namespaces: `ark8de` (apps), `ark8de-infra` (postgres, nats), `monitoring`, `argocd`, `ingress-nginx`

Secrets: start with k8s Secrets, migrate to Sealed Secrets (kubeseal) for git-safe encrypted secrets.

**Deliverable**: `kubectl get pods -n ark8de` shows everything running.

### Phase 6: GitOps with ArgoCD (Weeks 27-30)
**Goal**: Git is the source of truth. No manual `kubectl apply`.

- Install ArgoCD in `argocd` namespace
- Use "app of apps" pattern: root `Application` -> individual service `Application` manifests
- Kustomize overlays: `infra/k8s/overlays/dev/` and `prod/`
- GitHub Actions CD job: build -> push to `ghcr.io` -> update image tag in k8s manifests -> commit -> ArgoCD syncs

**Deliverable**: Push to `main` -> ArgoCD auto-deploys. Zero manual kubectl.

### Phase 7: Observability (Weeks 31-34)
**Goal**: Understand production behavior.

Each Go service exposes `/metrics` with:
- `http_requests_total{method, path, status_code}`
- `http_request_duration_seconds` (histogram)
- Domain metrics: `arkade_points_assigned_total`, `kredit_transfers_total`, `team_lock_ins_total`, `join_requests_total`
- `db_query_duration_seconds{query_name}`

Infrastructure:
- `kube-prometheus-stack` Helm chart (Prometheus + Grafana + AlertManager)
- `ServiceMonitor` CRDs for auto-discovery

Grafana dashboards:
1. App Overview: request rate, error rate, p99 latency per service
2. Leaderboard Activity: score submissions/hour, recalc rate
3. Database: connection pool, slow queries

Alerts: error rate > 5% for 5min, p99 latency > 2s, pod restarts > 3.

**Deliverable**: Live Grafana dashboard + at least one alert configured.

---

## Infrastructure Layers

Understanding what each tool owns prevents overlap and confusion:

| Layer | Tool | Owns |
|---|---|---|
| Cloud resources | **Terraform** | Cluster nodes, VPC, DNS, firewall |
| App deployment | **ArgoCD** | k8s Deployments, Services, ConfigMaps |
| Image builds | **GitHub Actions** | Docker images pushed to `ghcr.io` |
| App config | **k8s Secrets + Sealed Secrets** | Passwords, JWT keys, DB URLs |
| In-cluster infra | **Helm** | Postgres, NATS, ingress-nginx, Prometheus |

---

## Directory Structure

```
ark8de-l33tbo4rd/
├── README.md
├── ARCHITECTURE.md
├── Makefile
├── go.work                        # Go workspace (used in microservices phase)
│
├── monolith/                      # Phase 1: single binary
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── auth/       (handler.go, service.go, repository.go)
│   │   ├── team/
│   │   ├── character/
│   │   ├── match/
│   │   ├── leaderboard/
│   │   ├── middleware/
│   │   └── db/
│   ├── templates/                 # Go HTML templates
│   │   ├── layout/base.html
│   │   ├── leaderboard.html       # team rankings
│   │   ├── team-detail.html       # roster + gear pool status
│   │   ├── profile.html           # player class, skills, gear, stats, kredits
│   │   └── moderator.html         # assign points, grant kredits
│   ├── static/                    # htmx.min.js, style.css
│   ├── Dockerfile
│   └── go.mod
│
├── services/                      # Phase 3+: one dir per microservice
│   ├── auth/
│   ├── player/
│   ├── team/
│   ├── leaderboard/
│   ├── frontend/
│   └── gateway/
│
├── shared/pkg/                    # Shared Go code: jwt, middleware, errors, pagination
│
├── db/
│   └── migrations/                # All SQL migrations (source of truth)
│       ├── 000001_create_players.up.sql
│       ├── 000001_create_players.down.sql
│       └── ...
│
├── infra/
│   ├── docker-compose.yml
│   ├── terraform/
│   │   ├── environments/
│   │   │   ├── dev/               # main.tf, variables.tf, outputs.tf, terraform.tfvars (gitignored)
│   │   │   └── prod/
│   │   └── modules/
│   │       ├── cluster/           # k8s cluster (provider-agnostic)
│   │       ├── networking/        # VPC, subnets, firewall
│   │       └── dns/               # Domain A records
│   ├── k8s/
│   │   ├── base/                  # Per-service: deployment, service, configmap, hpa
│   │   └── overlays/dev/ prod/    # Kustomize patches
│   ├── argocd/
│   │   ├── install.yaml
│   │   └── applications/          # root-app.yaml + per-service apps
│   └── monitoring/
│       ├── prometheus-values.yaml
│       ├── grafana-dashboards/
│       └── alerting-rules.yaml
│
├── scripts/
│   ├── seed.go
│   └── gen-jwt-secret.sh
│
└── .github/workflows/
    ├── ci.yml        # PR: test, lint, build
    ├── cd.yml        # Merge to main: build, push, update manifests
    ├── infra.yml     # PR: terraform fmt/validate/plan; Merge: terraform apply
    └── db-migrate.yml
```

---

## Key Design Decisions

- **Monolith first**: Clean internal package boundaries allow Phase 3 extraction to be mechanical refactoring, not redesign
- **sqlc over ORM**: Write raw SQL, generate type-safe Go — learn SQL properly with compile-time safety
- **chi over Gin/Echo**: Uses stdlib `net/http` exclusively — middleware is reusable anywhere
- **HTMX + Go templates**: Full interactivity without a JS framework; everything stays in Go
- **NATS over Kafka**: Single binary, ~20MB, great Go client — right scale for this project
- **Leaderboard isolated from day 1**: Different access pattern (heavy read, eventual write) — scales independently
- **KIND for local k8s**: Free, instant (~30s), full Kubernetes in Docker — no cloud account needed to learn Terraform + k8s together
- **Terraform modules abstract the provider**: `modules/cluster/` hides whether the backend is KIND or a cloud — swapping dev to prod is changing one module source line
- **Terraform for infra, ArgoCD for apps**: Terraform provisions the cluster; ArgoCD manages what runs inside it — mixing both leads to drift and confusion
- **Remote state from day 1**: Terraform Cloud free tier handles state locking and history — local `.tfstate` breaks as soon as CI also runs Terraform

---

## Verification Approach

- **Phase 0**: `curl http://localhost:8080/healthz` returns 200; `docker compose ps` shows all containers healthy
- **Phase 1**: End-to-end browser test — register, set class, allocate skills, select gear, create team, send/accept join request, moderator assigns Arkade points, leaderboard updates
- **Phase 2**: `go test ./...` passes in CI; Docker image builds successfully in Actions
- **Phase 3**: Kill one service container, verify other services continue working; restart it, verify recovery
- **Phase 4**: `terraform apply` completes with no errors; `kubectl get nodes` shows cluster nodes Ready
- **Phase 5**: `kubectl get pods -n ark8de` all Running; app accessible via Ingress IP
- **Phase 6**: Make a code change, push to main, watch ArgoCD UI sync and rollout without touching kubectl
- **Phase 7**: Generate test traffic via script, confirm metrics appear in Grafana within 30s
