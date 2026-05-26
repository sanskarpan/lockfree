# Operations Guide

## Service Model

- Authentication: session cookie for browser users, bearer token for admin automation
- Authorization: `viewer`, `operator`, and `admin` roles scoped to a tenant
- Persistence: atomic JSON snapshot plus rotating timestamped backups
- Recovery: load primary snapshot first, then fall back to the newest valid backup
- Deployment topology: single-replica `StatefulSet` with persistent local storage; horizontal scaling requires an external shared state layer and is not supported by the current app design

## SLOs

- Availability: `99.9%` monthly for `/readyz`
- Recovery point objective: last persisted snapshot plus the most recent successful backup
- Recovery time objective: under `15 minutes` with valid snapshot or backup available

## Alert Expectations

- Page immediately on `LockfreeVisualizerUnavailable`
- Investigate within one business hour on persistence failures
- Investigate the same day on auth spikes or sustained rate limiting

## Backup and Recovery

1. Trigger an on-demand backup with `POST /api/v1/admin/backup` using an admin session or `LOCKFREE_ADMIN_API_TOKEN`.
2. Preserve `/app/data/state.json` and `/app/data/backups/`.
3. To restore, replace `state.json` with a known-good backup and restart the service.

## Validated Deployment Path

1. Build the container image from the repo `Dockerfile`.
2. Run the image locally with a real users file, session secret, and admin token.
3. Validate `/livez`, `/readyz`, session login, `/api/v1/session`, and `/metrics`.
4. Deploy the checked-in Kubernetes manifest to a `kind` cluster and repeat the same probe and auth flow through the service.
5. Exercise `.github/workflows/security.yml` locally with `act` to verify the checked-in CI jobs still run against the current repo state.

The repository has been exercised through that path, not just statically configured for it.

## Helm Deployment

Use the Helm chart in `deploy/helm/lockfree` when you want a parameterized deployment that still matches the current stateful topology.

```bash
helm install lockfree ./deploy/helm/lockfree \
  --namespace lockfree \
  --create-namespace
```

The chart intentionally deploys a single-replica `StatefulSet` with persistent local storage. It is not safe to scale horizontally unless the application gains an external shared state layer.

## Soak Automation

The repository includes a scheduled soak workflow in `.github/workflows/soak.yml`.

- It runs weekly on Sunday at 02:00 UTC.
- It also supports manual dispatch.
- It increases the authenticated soak test workload using `LOCKFREE_SOAK_WORKERS=12` and `LOCKFREE_SOAK_ITERATIONS=250`.
- It keeps the extended load test separate from the fast CI path so pull-request validation stays responsive.

## Observability

- Prometheus alert rules live in `deploy/monitoring/prometheus-alerts.yaml`.
- Grafana dashboard definitions live in `deploy/monitoring/grafana-dashboard.json`.
- Alertmanager routing lives in `deploy/monitoring/alertmanager.yml`.

The dashboard tracks the service-ready gauge, active WebSocket connections, snapshot freshness, tenant count, HTTP 5xx rate, authentication failures, rate-limit rejections, persistence operations, and WebSocket error volume.

The default Alertmanager routing splits notifications by severity so `critical`, `high`, and `medium` alerts can be pointed at different webhook endpoints or downstream notification systems.

## Secret Rotation

1. Generate a new `LOCKFREE_SESSION_SECRET`.
2. Move the previous secret into `LOCKFREE_PREVIOUS_SESSION_SECRETS`.
3. Roll out the new secret.
4. After the previous session TTL has elapsed, remove the old secret.
5. Rotate `LOCKFREE_ADMIN_API_TOKEN` and user password hashes independently.

## Incident Response

- If `/readyz` fails: inspect logs, disk permissions, and snapshot write errors.
- If login failures spike: verify rate-limit metrics and audit for brute-force traffic.
- If tenants observe cross-tenant state: treat as severity 1 and disable external access until confirmed fixed.
