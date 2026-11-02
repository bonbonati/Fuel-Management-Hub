# CLAUDE.md

Guidance for Claude Code (and other contributors/agents) working in this repository.

## Project

PetroFleet Sudan (بترو): fuel management for fleets and partner stations in Sudan.
Bilingual (Arabic RTL default, English), amounts in SDG (ج.س). Go microservices + React
web admin, containerized, deployable to Kubernetes. The original single-file prototype in
`prototype/index.html` is the UX reference (do not edit it; port from it).

**Mobile app is a planned stage** — `frontend/apps/mobile/README.md` has the decided stack
(React + Capacitor reusing `@petro/shared`). Don't build it unless asked.

## Branches

`dev` is the development branch; `main` is production (CI publishes images from `main`).
Work on `dev` (or feature branches into `dev`). Don't commit to `main` directly.

## Run & test

```bash
make test            # Go vet + tests (race) and TS typecheck + Vitest — run before every commit
make up              # docker compose stack → http://localhost:8080 (demo password demo1234)
make e2e             # Go end-to-end suite against the running stack
make e2e-live        # web UI smoke test against the running stack
make k8s-validate    # kustomize render + kubeconform (no cluster needed)
```

- Go: the module pins `toolchain go1.27.3` (auto-downloaded). Use `GOTOOLCHAIN=go1.27.3 go …` if your local Go is older.
- Frontend: pnpm 12 via `npx -y pnpm@12.16.1` (see Makefile). The user's `~/.npmrc` may point at an
  unreachable corporate proxy; if installs hang, run with `npm_config_userconfig=<file with only registry=https://registry.npmjs.org/>`. Never edit `~/.npmrc`.
- Prefer docker compose for local verification. Do **not** start k3s or deploy to a cluster
  unless explicitly asked (the dev machine also hosts other workloads).

## Unit tests are mandatory

Every change ships with tests (see `.claude/skills/unit-testing/SKILL.md`; the
`test-writer` agent can write them). In short:
- Go: pure rules → table tests; use cases → in-memory `Store` fakes (`memstore_test.go`,
  `fakes_test.go`); HTTP → `authntest` (real JWTs + role guards). No DB needed for unit tests.
- Frontend: pure helpers → Vitest; components/pages → Testing Library against the fake
  gateway in `apps/web/test/fixtures.ts` (real providers + API client).
- Cross-service behaviour → `backend/e2e` (build tag `e2e`).

## Backend (`backend/`, one Go module)

```
cmd/<svc>/main.go         composition root (config → db → wiring → app.Serve)
internal/<svc>/           model.go · rules.go (pure) · service.go (use cases over Repo/Store)
                          store_pg.go (pgx) · http.go (routes + role guards)
                          migrations/*.sql (goose, embedded) · seed.sql (idempotent demo data)
internal/platform/        app, authn (+authntest), config, db, events, httpx, i18n, kv, svcclient
internal/common/          shared domain primitives (Fuel, Plate, money bounds)
```

Services: **auth** (users, EdDSA JWT, JWKS) · **fleet** (org, tiers, drivers, vehicles,
limits, rules, requests) · **station** (stations, pumps, staff, prices) · **payment**
(wallet ledger, atomic charge/refund, VAT, settlements, event consumer) · **fueling**
(orchestrates fill-ups, pass tokens, scans, transactions, security log, outbox relay).
Read `docs/architecture.md` before changing cross-service behaviour.

Conventions:
- Ownership: a service only touches its own database. Cross-service reads go through
  `/internal/v1/...` endpoints via `svcclient` (forwards the user token); async side effects
  go through the outbox (`events.Enqueue` inside the same DB transaction) — never publish in a request.
- Authorization: wrap routes with `authn.Require(handler, roles...)`; take tenant/actor ids
  from `authn.Caller(r)` claims, never from the body. Rows of another tenant → 404.
- Errors: return `*httpx.Error` with a stable snake_case `code` and a bilingual message
  (`httpx.BadRequest/Conflict/...`). Unknown errors become a generic 500.
- Read-modify-write runs in `Store.InTx`; Postgres repos lock rows (`FOR UPDATE`) inside transactions.
- New table/column → new numbered migration file (never edit an applied one). Demo data → `seed.sql` with `ON CONFLICT DO NOTHING`.
- Money is `int64` whole SDG. Bilingual values are `i18n.Text` (JSONB `{ar,en}`).
- Comment every package and exported identifier (godoc); explain *why* for non-obvious rules.
- `gofmt` clean; `go vet` clean.

## Frontend (`frontend/`, pnpm workspace)

- `packages/shared`: API client + types (mirror the Go JSON), i18n (`useI18n` → `T(ar,en)`,
  `N(text)`), formatting (`sar`, `short`, `curTxt`, `plateTxt`, `whenLabel`…), domain helpers
  (`vatOf`, `tierStats`, `fleetAlerts`…), data hooks (`use*` TanStack Query, polling every 4 s),
  `useAction` for mutations (toast + refetch), UI atoms, shared fleet/station features, and
  `styles/petro.css` (the prototype's design system).
- `apps/web`: fleet portal (`fleet.tsx`), station portal + settings (`station.tsx`),
  declarative edit dialogs (`modals.tsx`, port of the prototype's `MODALS`), shell (`App.tsx`).
  Page state is in `location.hash`.
- Every user-visible string needs Arabic and English (`T('…','…')`, Arabic first).
- Reuse the prototype's markup/class names so `petro.css` applies; colors by role: emerald
  driver, blue fleet, orange station, violet/cyan data. `--white` is the darkest ink. RTL-first:
  prefer logical CSS properties. Light theme only.
- Authoritative rules (limits, caps, authorization) live in the backend; the frontend only derives display values.

## Deployment

- `compose.yaml` + `deploy/compose/traefik/dynamic.yml` — local stack (Traefik gateway on :8080).
- `deploy/k8s/base` — app workloads (non-root, read-only rootfs, probes, PDBs, NetworkPolicy,
  Ingress for Traefik); `components/infra` — Postgres/Valkey/RabbitMQ for dev/self-hosted;
  `overlays/dev` (k3s, demo on, generated dev secrets) and `overlays/prod` (registry images,
  HPAs, TLS, external `petro-secrets`, see `secrets.example.env`).
- When adding a service: Dockerfile build-arg, compose service + Traefik route, k8s base
  manifest + Ingress path, CI publish matrix, and docs.
