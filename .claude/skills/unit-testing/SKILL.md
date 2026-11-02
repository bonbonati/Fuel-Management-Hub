---
name: unit-testing
description: How to write and run the mandatory unit tests in this repo (Go services with in-memory fakes and authntest; React with Vitest + Testing Library against the fake gateway). Use whenever adding or changing backend or frontend code, or when asked to add tests.
---

# Unit testing in PetroFleet

Tests are mandatory for every change. A change is done when `make test` passes.

## Go (backend/)

Run: `cd backend && GOTOOLCHAIN=go1.27.3 go test -race ./...` (single package: `go test ./internal/fleet/`).

Layers and how to test each:

1. **Pure rules** (`rules.go`, `internal/common`): table-driven tests covering boundaries
   (e.g. exactly at the limit, one over), each error `code`, and both languages being present.
   Assert on `httpx.AsError(err).Code`, not on message text.
2. **Use cases** (`service.go`): construct the service over the package's in-memory store
   (`newSeededStore()` in fleet `memstore_test.go`, `memStore` in payment/fueling, `memRepo`
   in station/auth) and fakes for upstream services (`fueling/fakes_test.go`). The memory
   stores commit `InTx` only on success — assert that failed operations leave no partial writes.
   Cover tenant isolation (another org/station's ids → `not_found`).
3. **HTTP** (`http.go`): `env := authntest.New(t)`, `c := env.Server(NewAPI(svc).Register)`,
   `tok := env.Token(authn.RoleDriver, "o1", "", "d3")`, then `c.Do(method, path, tok, body, &out)`.
   Table-test the role guard of every new route (401 anonymous, 403 wrong role, 200/201 right role).
4. **Platform code**: plain unit tests; use `kv.NewMemory()` (inject `Now` to test TTLs).

Cross-service flows (charge → settlement event, QR scan → dispense) belong in
`backend/e2e/e2e_test.go` (`//go:build e2e`, runs against `docker compose up`; must stay
re-runnable: assert deltas, restore what you change).

## TypeScript (frontend/)

Run: `cd frontend && npx -y pnpm@12.16.1 -r typecheck && npx -y pnpm@12.16.1 -r test`.

- Pure helpers in `packages/shared/src` (format, domain): Vitest unit tests in `packages/shared/test`.
- Hooks/providers: `renderHook` with `AppProviders` as wrapper (see `react.test.tsx`).
- Web pages: render `<AppProviders storageKey="petro-web-session"><App/></AppProviders>` with
  `fakeGateway({...overrides})` from `apps/web/test/fixtures.ts`, sign in with `signIn(users.fm)`,
  pick the page via `window.location.hash`, drive it with `userEvent`, and assert both the API
  call (`callsTo(fetch, 'PATCH', path)`) and the visible result (toast `role="status"`).
  Query by role/label (accessible names), in English (`signIn` sets `petro-lang=en`).
- New API endpoint → add it to the fake gateway defaults if pages read it on load.

## Checklist

- [ ] Happy path + every validation/permission branch you added
- [ ] Boundaries for money/limits
- [ ] No partial writes on failure (service layer)
- [ ] Role guard table for new routes
- [ ] `make test` green; `gofmt -l backend` empty
