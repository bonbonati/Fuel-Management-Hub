# PetroFleet Sudan · بترو

Fuel-management platform for Sudanese fleets and partner stations: fleet managers set
budgets, tiers and anti-fraud rules; drivers fuel with a rotating QR pass; station
attendants identify vehicles by QR or RFID and dispense; everything is charged to the
company's prepaid wallet. Bilingual (Arabic RTL by default, English), amounts in SDG (ج.س).

> منصة لإدارة الوقود تربط السائق ومدير الأسطول والمحطة. الواجهة ثنائية اللغة (العربية افتراضياً)
> والمبالغ بالجنيه السوداني. البيانات التجريبية تُزرع تلقائياً في البيئة المحلية.

## Status

| Part | State |
|---|---|
| Backend — 5 Go microservices | ✅ implemented, unit + e2e tested |
| Web admin (fleet + station portals) | ✅ implemented, unit tested |
| Local environment (docker compose) | ✅ one command |
| Kubernetes (kustomize: base, infra component, dev/prod overlays) | ✅ manifests, schema-validated in CI |
| Mobile app (driver / fleet manager / attendant) | 🕓 planned — see `frontend/apps/mobile/README.md` |

The original single-file clickable prototype is kept in `prototype/index.html` as the UX reference.

## Quick start

```bash
docker compose up --build -d     # or: make up
open http://localhost:8080       # web admin
```

Sign in with the one-tap demo buttons, or with a username below (password `demo1234`):

| Username | Role | Used in |
|---|---|---|
| `fleet.manager` | Fleet manager (Al Namaa Logistics) | web admin |
| `ATT-01` | Station supervisor (Blue Nile Station) | web admin |
| `D-3071` | Driver (Abdurahman Hassan) | mobile app (planned) / API |
| `ATT-09` | Pump attendant | mobile app (planned) / API |

Other local endpoints: Traefik dashboard http://localhost:8090 · RabbitMQ console
http://localhost:15672 (`petro` / `petro-dev`) · Postgres `localhost:55432`.

## Repository layout

```
backend/            Go module: all microservices (cmd/<service>, internal/<service>, internal/platform)
  e2e/              end-to-end tests against a running stack (build tag e2e)
frontend/           pnpm workspace
  packages/shared/  API client + types, i18n, formatting, domain helpers, UI atoms, design-system CSS
  apps/web/         web admin (React 19 + Vite 8)
  apps/mobile/      planned mobile app (README only)
deploy/
  compose/          gateway (Traefik) routing for docker compose
  k8s/              kustomize: base/, components/infra/, overlays/{dev,prod}
docs/               architecture and decisions
prototype/          original HTML prototype
compose.yaml        local environment
Makefile            make help
```

## Everyday commands

```bash
make test            # all unit tests (Go + TypeScript)
make up / make down  # local stack
make e2e             # end-to-end suite against the running stack
make e2e-live        # web UI smoke test against the running stack
make k8s-validate    # render + schema-validate the k8s overlays
```

## Branches

- `dev` — integration branch; all work lands here via PRs.
- `main` — production. CI publishes images to GHCR on every push to `main`.

See [docs/architecture.md](docs/architecture.md) for the design and [CLAUDE.md](CLAUDE.md) for contributor conventions.
