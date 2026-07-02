# Phase 5 Complete: Kubernetes

## What was built

The full stack now runs on the Terraform-provisioned KIND cluster.

**Per service** (all six): `ConfigMap` (ports, NATS URL, gateway upstreams) + `Deployment` (readiness/liveness probes on `/healthz`, resource requests/limits) + `Service` (ClusterIP) + `HPA` (autoscaling/v2, 1–3 replicas at 70% CPU, fed by metrics-server). Manifests in `infra/k8s/base/`.

**Shared infra** (Helm, values in `infra/k8s/helm-values/`):
| Component | Chart | Namespace |
|---|---|---|
| PostgreSQL | Bitnami `postgresql` | `ark8de-infra` |
| NATS JetStream | `nats/nats` | `ark8de-infra` |
| ingress-nginx | `ingress-nginx` (KIND mode: hostPort on the ingress-ready node) | `ingress-nginx` |
| metrics-server | `metrics-server` (`--kubelet-insecure-tls` for KIND) | `kube-system` |
| Sealed Secrets | controller from GitHub release manifest | `kube-system` |

**Secrets**: `JWT_SECRET` + `DATABASE_URL` live in a `SealedSecret` (`infra/k8s/base/app-secrets-sealed.yaml`) — encrypted with the cluster controller's public key, safe to commit; the plain Secret never touches git. Note: sealed data is bound to this cluster's key — re-seal with `kubeseal` after recreating the cluster.

**Jobs**: `db-migrate` (runs baked-in `db/migrations`, services deploy after it completes) and `db-seed` (demo data, run once on a fresh DB).

## Traffic & topology

```mermaid
flowchart LR
    User[localhost:8880] --> IN[ingress-nginx<br/>hostPort on control-plane]
    IN --> GW[gateway Service]
    GW --> AUTH[auth] & PLAYER[player] & TEAM[team] & LB[leaderboard] & FE[frontend]
    subgraph ark8de-infra
      PG[(PostgreSQL<br/>Bitnami Helm)]
      NATS[NATS JetStream Helm]
    end
    AUTH & PLAYER & TEAM & LB & FE --> PG
    PLAYER & TEAM --) NATS
    NATS --) LB
```

Namespaces: `ark8de` (apps + jobs), `ark8de-infra` (postgres, nats), `ingress-nginx`, `kube-system` (metrics-server, sealed-secrets).

## Deploy from scratch

```bash
cd infra/terraform/environments/dev && terraform apply          # cluster
for s in auth-service player-service team-service leaderboard-service frontend-service gateway migrate seed; do
  docker build -f monolith/Dockerfile --build-arg SERVICE=$s -t ark8de/$s:dev . && kind load docker-image ark8de/$s:dev --name ark8de-dev
done
helm install postgres oci://registry-1.docker.io/bitnamicharts/postgresql -n ark8de-infra -f infra/k8s/helm-values/postgres-values.yaml
helm install nats nats/nats -n ark8de-infra -f infra/k8s/helm-values/nats-values.yaml
helm install ingress-nginx ingress-nginx/ingress-nginx -n ingress-nginx --create-namespace -f infra/k8s/helm-values/ingress-nginx-values.yaml
helm install metrics-server metrics-server/metrics-server -n kube-system --set 'args={--kubelet-insecure-tls}'
kubectl apply -f https://github.com/bitnami-labs/sealed-secrets/releases/latest/download/controller.yaml
kubectl apply -f infra/k8s/base/   # namespaces, sealed secret, jobs, services, ingress
```

## Verified

- `kubectl get pods -n ark8de` — all 6 services `Running`, both jobs `Completed`.
- HPAs report live CPU (e.g. `cpu: 10%/70%`) via metrics-server.
- E2E through ingress at `http://localhost:8880`: healthz/leaderboard/login → 200; moderator login → award points → `team.points_updated` consumed by leaderboard-service inside the cluster.

## Next phase

Phase 6: ArgoCD + app-of-apps + Kustomize overlays — git becomes the deployment source of truth (`infra/k8s/base/` is already structured for Kustomize).
