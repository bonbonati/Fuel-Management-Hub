# Project status (handoff)

_Last updated: 2026-11-02. Keep this page current: update it at the end of every
work session so the next person or agent can pick up immediately._

## Where we are

The single-file HTML prototype (`prototype/index.html`) has been re-platformed:

| Area | State | Notes |
|---|---|---|
| Backend: auth, fleet, station, payment, fueling (Go 1.27) | ✅ done | Unit tests per layer + `backend/e2e` suite (passes against compose) |
| Web admin: fleet + station portals (React 19, Vite 8) | ✅ done | Unit tests + opt-in live smoke test (`make e2e-live`) |
| Local environment (`compose.yaml`, Traefik gateway :8080) | ✅ done | Runs on Docker Desktop from a fresh clone (verified by CI e2e on a clean runner); guide: `docs/local-setup.md` |
| Kubernetes (kustomize base / infra component / dev & prod overlays) | ✅ manifests | Schema-validated (`make k8s-validate`); **not yet deployed to a real cluster** |
| CI (`.github/workflows/ci.yml`) | ✅ | dev pushes + PRs: tests, web build, overlay validation, compose e2e |
| CD (`.github/workflows/cd.yml`) | ✅ waiting for config | On main: build + push images `$REGISTRY/<svc>:<sha>`, pin tags in `deploy/k8s/overlays/prod`, commit; Argo CD syncs |
| Argo CD | 🕓 owner to set up | Example: `deploy/argocd/application.yaml` |
| Mobile app | 🕓 planned | `frontend/apps/mobile/README.md` (React + Capacitor, reuse `@petro/shared`) |

## To go live (owner actions)

1. GitHub repo → Settings → Secrets and variables → Actions:
   variable `REGISTRY` (e.g. `registry.example.com/petro`), secrets `REGISTRY_USERNAME`, `REGISTRY_PASSWORD`.
   Until `REGISTRY` is set, CD is skipped with a notice.
2. In the cluster: Secret `petro-secrets` in namespace `petro`
   (keys in `deploy/k8s/overlays/prod/secrets.example.env`, incl. a persistent Ed25519 signing key);
   reachable Postgres (5 databases/roles, see `deploy/k8s/components/infra/init-databases.sh`),
   RabbitMQ and Valkey — or add `../../components/infra` to the prod overlay to self-host them.
3. Adjust the Ingress host/TLS patch in the prod overlay (`petro.example.com`, cert-manager issuer)
   and the NetworkPolicy's ingress-controller namespaces if needed.
4. `kubectl apply -n argocd -f deploy/argocd/application.yaml`.

## Next candidates

- Mobile app (driver, fleet manager, attendant).
- Enforce anti-siphoning, geofence and fueling-hour rules server-side.
- Snapshot VAT on transactions; PDF receipts/statements (then add object storage).
- Monthly reset of approved extras; refresh tokens; provision auth accounts for new drivers via an event.
- Tracing (OpenTelemetry).

## Decisions log

- 2026-11-02 — Go microservices, database per service, EdDSA JWT + JWKS, transactional outbox
  to RabbitMQ, Valkey for ephemeral state; Kafka and S3 deferred (see `architecture.md`).
- 2026-11-02 — Web admin in React; mobile deferred to a later stage (Capacitor chosen).
- 2026-11-02 — Branching: `dev` for development, `main` = production (GitOps via Argo CD).
- 2026-11-02 — Local verification uses docker compose only; don't start k3s on the dev machine, and avoid heavy
  rebuilds there (e.g. `--no-cache`, fresh-clone builds) — rely on CI for clean-environment checks.
- 2026-11-02 — Host ports configurable via `.env`; `.gitattributes` forces LF (Windows checkouts).
