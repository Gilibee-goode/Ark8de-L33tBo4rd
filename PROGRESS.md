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

## Phase 2: Testing + CI
- [x] Extract shared router into `internal/app/router.go` (used by both server and tests)
- [x] Integration test suite with `httptest` + real PostgreSQL (55 test cases across 5 files)
- [x] Unit tests for all service layer functions (66 tests across 3 files — auth, player, team)
- [ ] DB tests with `testcontainers-go`
- [ ] GitHub Actions — `go vet`, `go test`, `golangci-lint`
- [ ] Structured logging with `log/slog`
- [ ] Multi-stage Docker build (target < 20MB)
- [ ] `docs/phase-2-complete.md` with diagrams

## Phase 3: Service Extraction
- [ ] Extract leaderboard-service
- [ ] Extract auth-service
- [ ] Extract player-service + NATS JetStream events
- [ ] Extract team-service
- [ ] Add api-gateway
- [ ] Update docker-compose with all services + NATS
- [ ] `docs/phase-3-complete.md` with diagrams

## Phase 4: Terraform + KIND
- [ ] Install KIND + Terraform locally
- [ ] `modules/cluster` with KIND provider
- [ ] `environments/dev` wired to KIND module
- [ ] Terraform Cloud remote state configured
- [ ] `terraform apply` provisions cluster
- [ ] `infra.yml` GitHub Actions workflow
- [ ] `docs/phase-4-complete.md` with diagrams

## Phase 5: Kubernetes
- [ ] Deployment + Service + ConfigMap + HPA per service
- [ ] PostgreSQL via Bitnami Helm chart
- [ ] NATS via Helm chart
- [ ] ingress-nginx configured
- [ ] Sealed Secrets for sensitive config
- [ ] All pods Running in `ark8de` namespace
- [ ] `docs/phase-5-complete.md` with diagrams

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
