# @petro/mobile — planned (not implemented yet)

The mobile app is a **future stage**. The original prototype's three phone apps
(driver, fleet manager, station attendant) live in `prototype/index.html` and are
the reference for the port.

## Decided stack

- **React + Vite + TypeScript**, packaged for Android/iOS with **Capacitor 8**.
  The prototype's mobile UI is HTML/CSS, so the shared stylesheet
  (`packages/shared/src/styles/petro.css`, which already contains the phone,
  dock and sheet styles) is reused as-is instead of rewriting the UI natively.
- **One app, role-based shells** after sign-in (driver / fleet manager /
  attendant), like the prototype's role switcher.
- Reuses `@petro/shared`: API client and types, `AppProviders`, `Login`,
  data hooks, `useFleetData` + fleet cards (request, tier, rules, alerts),
  `useStationData`, `useTxActions`, `QrCode`, i18n and formatting.
- QR scanning: `@capacitor/barcode-scanner` in native builds; manual token
  entry in the browser. RFID is simulated (tag → vehicle `f6`) as in the prototype.
- Native builds call the gateway at `VITE_API_BASE_URL`; add the app origins
  (`capacitor://localhost`, `https://localhost`) to `CORS_ALLOWED_ORIGINS`.

## Backend support already in place

Every endpoint the phone apps need exists and is covered by unit + e2e tests:
`GET /api/v1/fleet/me`, `GET /api/v1/fueling/me/pass`, `POST /api/v1/fueling/me/pass-token`
(30 s rotating token), `POST /api/v1/fueling/me/odometer`, `POST /api/v1/fueling/authorizations`,
`POST /api/v1/fleet/requests`, `POST /api/v1/fueling/scans`, `POST /api/v1/fueling/dispenses`,
`POST /api/v1/fueling/transactions/start-by-code`, `…/{id}/start`, `…/{id}/finish`.

## When implementing

1. Scaffold like `apps/web` (package.json, vite.config.ts with `base: './'`,
   capacitor.config.ts), session storage key `petro-mobile-session`.
2. Add a `mobile` service to `compose.yaml`, a `/m` route to
   `deploy/compose/traefik/dynamic.yml` and to `deploy/k8s/base/ingress.yaml`.
3. Unit tests with Vitest + Testing Library against a fake gateway (see `apps/web/test/fixtures.ts`).
