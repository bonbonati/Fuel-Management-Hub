# Run PetroFleet locally (Docker Desktop)

Everything runs in containers, so the only requirement is Docker. No Go, Node or database
installation is needed to run the app.

## Requirements

| | |
|---|---|
| Docker | **Docker Desktop** (Windows, macOS Intel/Apple Silicon) or Docker Engine + Compose v2 on Linux |
| Resources | ≈ 4 GB RAM and ≈ 5 GB disk free for images and volumes; give Docker Desktop at least 4 GB (Settings → Resources) |
| Network | Internet access on the first build (base images, Go modules, npm packages) |
| Free ports | `8080` (app), `8090`, `15672`, `55432` — all changeable, see [Ports](#ports) |

Windows: use Docker Desktop with the WSL 2 backend. The repository forces LF line endings
(`.gitattributes`), so a normal `git clone` works even with `core.autocrlf=true`.

## Start

```bash
git clone https://github.com/bonbonati/Fuel-Management-Hub.git
cd Fuel-Management-Hub
docker compose up --build -d
```

The first build takes a few minutes (it compiles five Go services and the web app); later
starts take seconds. Wait until everything is healthy:

```bash
docker compose ps        # every service should show "(healthy)"
```

or start with `docker compose up --build -d --wait` to block until healthy.

Then open **http://localhost:8080**.

## Sign in

Use the one-tap **demo** buttons on the sign-in page, or a username with password `demo1234`:

| Username | Role | What to try |
|---|---|---|
| `fleet.manager` | Fleet manager — Al Namaa Logistics | Overview, approve driver requests, vehicles/drivers/tiers, fuel rules, VAT, alerts |
| `ATT-01` | Station supervisor — Blue Nile Station | Live operations (start/finish fill-ups), pumps, staff, settlements |

The sidebar's portal switch moves between the two personas. Driver (`D-3071`) and attendant
(`ATT-09`) accounts exist for the API and the upcoming mobile app.

The database is seeded with the prototype's demo data on first start (6 vehicles, 3 tiers,
3 stations, transactions, a pending request…). Changes you make are kept between restarts.

## Try the whole flow without the mobile app

The driver and attendant apps are not built yet, but their API is. With the stack running,
the end-to-end suite plays a full journey (driver authorizes → attendant starts and finishes
→ station settlement updated → request approved → suspension and reactivation), and you can
watch the results appear in the web admin:

```bash
# needs Go installed (any recent version; the right toolchain is fetched automatically)
cd backend && go test -count=1 -tags e2e ./e2e/ -v
```

## Useful URLs

| URL | What |
|---|---|
| http://localhost:8080 | Web admin (and the API under `/api/v1/...`) |
| http://localhost:8090 | Traefik gateway dashboard |
| http://localhost:15672 | RabbitMQ console — `petro` / `petro-dev` |
| `localhost:55432` | Postgres — user `postgres` / `postgres-dev`; one database per service (`auth`, `fleet`, `station`, `payment`, `fueling`) |

## Ports

If a port is taken, create a `.env` file next to `compose.yaml` (see `.env.example`):

```dotenv
GATEWAY_PORT=9080
TRAEFIK_DASHBOARD_PORT=9090
RABBITMQ_UI_PORT=25672
POSTGRES_PORT=55433
```

then `docker compose up -d` again and open http://localhost:9080.

## Everyday commands

```bash
docker compose ps                          # status
docker compose logs -f fleet fueling       # follow service logs
docker compose up --build -d               # rebuild after pulling new code
docker compose down                        # stop (data kept)
docker compose down -v                     # stop and DELETE all data (fresh demo data next start)
```

With `make` available (Linux/macOS/WSL): `make up`, `make down`, `make test`, `make e2e`, `make help`.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `port is already allocated` | Change the port in `.env` (see [Ports](#ports)). |
| A service stays `unhealthy` / restarts | `docker compose logs <service>`. Services wait for Postgres and RabbitMQ, so the first start can take up to a minute. |
| Signed out after a restart | Expected in local mode: the auth service creates a new signing key on each start. Sign in again. To keep sessions, set `AUTH_SIGNING_KEY` in `.env` to an Ed25519 PEM key (`openssl genpkey -algorithm ed25519`). |
| Want the original demo data back | `docker compose down -v && docker compose up -d` |
| Build fails downloading packages behind a corporate proxy | Configure the proxy in Docker Desktop (Settings → Resources → Proxies). |
| Slow machine | Close other heavy workloads; the stack itself needs ≈ 1 GB RAM when running, more while building. |

## Developing (optional)

To work on the code with hot reload instead of rebuilding containers, you also need
Go 1.27 and Node 24+: run `pnpm --filter @petro/web dev` in `frontend/` (proxies the API to
`http://localhost:8080`) while the compose stack runs. See [CLAUDE.md](../CLAUDE.md) for
conventions and [architecture.md](architecture.md) for the design.
