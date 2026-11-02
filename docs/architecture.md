# Architecture

## Overview

```
                    ┌──────────── gateway (Traefik / k8s Ingress) ────────────┐
 browser ─────────► │ /            → web      (nginx, static React SPA)       │
 (mobile, planned)  │ /api/v1/auth → auth     /api/v1/fleet    → fleet         │
                    │ /api/v1/stations → station  /api/v1/payments → payment   │
                    │ /api/v1/fueling  → fueling   (/internal/* never routed)  │
                    └──────────────────────────────────────────────────────────┘
   fueling ──HTTP /internal (user token forwarded)──► fleet, station, payment
   fueling ──outbox → RabbitMQ "fueling.transaction.completed"──► payment (settlements)
   every service ──JWKS (cached)──► auth
```

| Service | Owns | Store |
|---|---|---|
| **auth** | user accounts, sign-in, EdDSA JWT issuing, JWKS | Postgres `auth`, Valkey (login throttling) |
| **fleet** | org, tiers, drivers, vehicles, **limits**, rules, driver requests | Postgres `fleet` |
| **station** | stations, pumps, staff, fuel prices | Postgres `station` |
| **payment** | wallets + append-only ledger, **usage**, atomic charges/refunds, VAT, settlements | Postgres `payment`, RabbitMQ consumer |
| **fueling** | fill-up orchestration, transactions (auth → fueling → done), odometer checks, security log | Postgres `fueling`, Valkey (pass tokens, scan sessions), outbox → RabbitMQ |
| **web** | web admin SPA (fleet + station portals) | — |

## Key decisions

**Database per service.** Each service has its own Postgres database and role; no service
reads another's tables. Schemas are embedded goose migrations applied at start-up under an
advisory lock (safe with many replicas). Demo data is a separate idempotent seed script,
enabled only with `SEED_DEMO_DATA=true`.

**Stateless services.** No in-process state: sessions are JWTs, ephemeral state is in
Valkey, everything else in Postgres. Every service scales horizontally (HPAs in prod).

**Authentication.** auth signs short-lived tokens with Ed25519 (alg EdDSA) and publishes
the public key at `/.well-known/jwks.json`. Other services verify locally with a cached
JWKS (refetched on unknown `kid`, throttled), so auth is never on the hot path. Claims carry
role, tenant (org or station) and actor (driver/staff id); handlers authorize on claims,
never on ids from request bodies.

**Service-to-service calls** go to `/internal/v1/...` routes with the caller's token
forwarded, so the callee authorizes the same identity. Internal routes are not exposed by
the gateway/Ingress, and a NetworkPolicy limits pod ingress to the namespace and the
ingress controller.

**Money is consistent where it matters.** Limits live in fleet, usage in payment's ledger.
A charge is checked (remaining monthly limit, wallet balance) and recorded in one
transaction holding the wallet row lock, so concurrent fill-ups cannot overspend. Charges
and refunds are idempotent by transaction id; if fueling cannot record a transaction after
charging, it refunds (compensating action).

**Events.** Asynchronous side effects use a transactional outbox: the state change and the
event commit together, a relay publishes with publisher confirms, consumers dedupe by event
id. Queues are quorum queues with a delivery limit and a dead-letter queue.

**Errors** are RFC 9457 problem documents with a stable `code` and a bilingual `message`
`{ar, en}` that the apps show verbatim.

## Infrastructure evaluation

| Option | Decision | Why |
|---|---|---|
| **PostgreSQL 18** | ✅ used | Relational, transactional money ledger; row locks for atomic charges; JSONB for bilingual text. |
| **Valkey 9** (Redis-compatible) | ✅ used | Shared, TTL'd ephemeral state across replicas: 30 s QR pass tokens, scan sessions, login throttling. No persistence needed. Chosen over Redis for its BSD licence. |
| **RabbitMQ 4.4** | ✅ used | Reliable domain events between services (settlement accrual today; notifications next). Quorum queues + DLQ give at-least-once delivery with simple ops. |
| **Kafka** | ❌ not now | No event-streaming, replay or high-throughput analytics need; would add significant operational weight. Revisit for telemetry/analytics pipelines. |
| **S3 / object storage** | ❌ not yet | No stored files today. Needed when odometer photos and PDF receipts/statements are implemented (MinIO locally, S3-compatible in prod). |

## Known gaps (future work)

- Mobile app (see `frontend/apps/mobile/README.md`).
- Rules shown but not yet enforced server-side: anti-siphoning, geofence, fueling hours.
- VAT is computed for display from the current rate; receipts should snapshot it.
- Monthly reset of approved extras; refresh tokens; account provisioning for new drivers
  (e.g. auth consuming a `fleet.driver.created` event); PDF statements/receipts.
- Observability beyond `/metrics` and JSON logs (tracing with OpenTelemetry).
