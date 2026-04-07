# Ark8de-L33tBo4rd — Progress Tracker

## Phase 0: Foundation
- [ ] Init Go module
- [ ] Create repo directory structure
- [ ] `docker-compose.yml` (postgres + app + pgAdmin)
- [ ] SQL migrations (golang-migrate)
- [ ] Makefile (`dev`, `migrate-up`, `migrate-down`, `test`)
- [ ] Seed script scaffold
- [ ] `GET /healthz` returns 200
- [ ] `docs/phase-0-complete.md` with diagrams

## Phase 1: Monolith
- [ ] Auth — register, login, JWT, 4-tier role middleware
- [ ] Player profile — class_role, profile photo upload, computed stats
- [ ] Skills — seed skill data per role, allocation + point budget enforcement
- [ ] Gear — selection UI, team gear pool display (highlight negative)
- [ ] Teams — create, join requests, accept/reject, lock-in, logo upload
- [ ] Kredits — moderator grant, player-to-player transfer, history
- [ ] Arkade points — moderator assigns to teams, audit log
- [ ] Leaderboard — ranked by Arkade points
- [ ] Frontend — Go templates + HTMX (leaderboard, team detail, profile, moderator panel)
- [ ] `docs/phase-1-complete.md` with diagrams

## Phase 2: Testing + CI
- [ ] Unit tests for all service layer functions
- [ ] Integration tests with `httptest`
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
