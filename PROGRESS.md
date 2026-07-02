# Ark8de-L33tBo4rd — Progress Tracker

## Phase 0: Foundation
- [x] Init Go module (`monolith/go.mod` — module `github.com/Gilibee-goode/ark8de-l33tbo4rd`, Go 1.23)
- [x] Create repo directory structure
- [x] `docker-compose.yml` (postgres + app + pgAdmin) — `infra/docker-compose.yml`
- [x] SQL migrations (golang-migrate) — 11 migration pairs in `db/migrations/`
- [x] Makefile (`dev`, `migrate-up`, `migrate-down`, `test`, `build`, `seed`, `deps`, `help`)
- [x] Seed script scaffold (`scripts/seed.go` — Phase 1 will implement)
- [x] `GET /healthz` returns 200 — pings DB, returns 503 if unreachable
- [x] `docs/phase-0-complete.md` with Mermaid flow + ER diagrams

## Phase 1: Monolith ✅
- [x] Auth — register, login, JWT, 4-tier role middleware (`POST /auth/register`, `POST /auth/login`, `GET /auth/me`)
- [x] Player profile — class_role, computed stats (HP/armor/SP from class + skills)
- [x] Skills — 16 seeded skills (4 per class), allocation with SP budget enforcement
- [x] Gear — selection, team gear pool display, over-budget highlighted red
- [x] Teams — create, join requests, accept/reject, lock-in, member removal
- [x] Kredits — moderator grant, player-to-player transfer, transaction history
- [x] Arkade points — moderator assigns delta to teams, full audit log
- [x] Leaderboard — ranked by Arkade points via RANK() window function
- [x] Frontend — Go templates + dark theme CSS (leaderboard, team detail, profile, moderator panel)
- [x] Seed script — 2 full teams, 11 players, realistic gear + skill + Kredit data
- [x] `docs/phase-1-complete.md` with diagrams

## Phase 2: Testing + CI (GitHub Actions deferred)
- [x] Extract shared router into `internal/app/router.go` (used by both server and tests)
- [x] Integration test suite with `httptest` + real PostgreSQL (70 test cases across 6 files)
- [x] Unit tests for all service layer functions (74 tests across 4 files — auth, player, team, session)
- [x] DB tests with `testcontainers-go` (auto-launches PostgreSQL container, runs migrations, no manual setup)
- [ ] GitHub Actions — `go vet`, `go test`, `golangci-lint`
- [x] Structured logging with `log/slog` (JSON output via `slog.NewJSONHandler`, all handlers and services)
- [x] Multi-stage Docker build — scratch-based, 13.3 MB final image (target was < 20MB)
- [x] `docs/phase-2-complete.md` with diagrams

## Phase 2.5: GUI Login & Session System ✅
- [x] Server-side sessions in PostgreSQL (`sessions` table — migration 000013)
- [x] Session package (`internal/session/` — model, repository, service, service_test)
- [x] Cookie-based auth middleware (`AuthenticateWithSessions`, `OptionalAuthenticateWithSessions`)
- [x] CSRF protection via double-submit cookie (`internal/middleware/csrf.go`)
- [x] Flash messages via short-lived cookie (`internal/middleware/flash.go`)
- [x] PageContext struct — navbar shows login state, username, flash messages on all pages
- [x] Login page (`GET/POST /login`) — form-based auth, session cookie, redirect
- [x] Register page (`GET/POST /register`) — form-based signup, password confirmation
- [x] Logout (`POST /logout`) — session destruction, cookie clearing
- [x] Moderator panel migrated from JavaScript fetch() to HTML forms with CSRF
- [x] CSS additions — flash messages, auth cards, navbar user section, button variants
- [x] Session service unit tests (8 tests with mock repository)
- [x] Integration tests for login/register/logout/CSRF/navbar (15 new tests in `tests/session_test.go`)
- [x] Backward compatibility — all 55 existing integration tests pass (Bearer JWT still works)

## Phase 3: Service Extraction ✅
- [x] Extract leaderboard-service (subscribes to `team.>` events, durable NATS consumer)
- [x] Extract auth-service
- [x] Extract player-service + NATS JetStream events (`player.gear_updated`)
- [x] Extract team-service (publishes `team.points_updated`, `team.membership_changed`)
- [x] Add api-gateway (reverse proxy by prefix + JWT signature validation at the edge)
- [x] Add frontend-service (server-rendered HTML split out of the monolith)
- [x] Update docker-compose with all services + NATS (+ one-shot migrate job)
- [x] `docs/phase-3-complete.md` with diagrams
- Note: extraction uses the multi-binary monorepo pattern — one Go module, six
  `cmd/` binaries, one parameterized Dockerfile (`--build-arg SERVICE=...`).
  E2E verified: points award through gateway → NATS event → leaderboard-service.

## Phase 4: Terraform + KIND ✅ (minus GitHub Actions)
- [x] Install KIND + Terraform locally
- [x] `modules/cluster` with KIND provider (tehcyx/kind, ingress ports 8880/8443 mapped)
- [x] `environments/dev` wired to KIND module (1 control-plane + 2 workers, k8s v1.31)
- [ ] Terraform Cloud remote state configured — `cloud {}` block written but commented;
      needs a TFC account + `terraform login` (state is local until then)
- [x] `terraform apply` provisions cluster (`kubectl get nodes` → 3 Ready)
- [ ] `infra.yml` GitHub Actions workflow (deferred — GHA excluded from this pass)
- [x] `docs/phase-4-complete.md` with diagrams

## Phase 5: Kubernetes ✅
- [x] Deployment + Service + ConfigMap + HPA per service (probes on /healthz, metrics-server feeds HPAs)
- [x] PostgreSQL via Bitnami Helm chart (`ark8de-infra` namespace)
- [x] NATS via Helm chart (JetStream + file store PVC)
- [x] ingress-nginx configured (KIND hostPort mode → app at http://localhost:8880)
- [x] Sealed Secrets for sensitive config (controller + `app-secrets-sealed.yaml`; plain secret never committed)
- [x] All pods Running in `ark8de` namespace (+ db-migrate / db-seed Jobs Completed)
- [x] `docs/phase-5-complete.md` with diagrams

## Phase 6: GitOps with ArgoCD
- [ ] ArgoCD installed in `argocd` namespace
- [ ] App-of-apps pattern configured
- [ ] Kustomize overlays for dev/prod
- [ ] GitHub Actions CD pushes image + updates manifests
- [ ] ArgoCD auto-syncs on merge to `boss`
- [ ] `docs/phase-6-complete.md` with diagrams

## Phase 7: Observability
- [ ] `/metrics` endpoint on every service
- [ ] `kube-prometheus-stack` deployed
- [ ] ServiceMonitors configured
- [ ] Grafana dashboards (app overview, leaderboard activity, DB)
- [ ] At least one alert rule configured
- [ ] `docs/phase-7-complete.md` with diagrams
