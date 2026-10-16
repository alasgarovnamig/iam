# Wallet Gateway — Platform

A single API gateway that customer companies integrate with instead of wiring
up a dozen third-party providers themselves. They sign one contract with us and
consume every service through our API.

The platform is built to grow into wallet management, payments, cards, bank
integrations, KYC/KYB, transactions, payouts, pay-ins, refunds, reconciliation,
limits, fees, notifications, reporting and provider routing. **This repository
currently implements the foundation those services stand on**, production-ready:

| Delivered | Notes |
|---|---|
| User Management service | Tenants, users, roles, permissions, API clients, audit |
| Authentication | Email/password + OAuth 2.0 client credentials, rotating refresh tokens |
| Multi-tenancy | Application scoping **and** PostgreSQL Row-Level Security |
| Authorization | OpenFGA relationships + OPA policy + field-level permission engine |
| Dynamic search | AND/OR/operator condition trees with per-field `search.<field>` authorization |
| Permission delegation | You may only grant what you hold, at your scope or narrower |
| Audit log | Append-only, immutable at the database level |
| React admin portal | Permission-aware UI, HttpOnly refresh cookies |
| Infrastructure | PostgreSQL, Redis, Kafka, OPA, OpenFGA, Prometheus, Grafana, Jaeger |

Wallet, transaction, payment and provider-integration services are **not** part
of this repository yet. This repo is one service, laid out flat: one Go module,
one binary set, one frontend.

---

## Quick start

```bash
git clone <repository-url> && cd wallet-gateway
cp .env.example .env          # local defaults; never commit a real .env
docker compose up --build     # first run pulls images and builds, ~3–5 minutes
```

Then open the portal at **http://localhost:3000** and sign in with the
bootstrap administrator from your `.env`.

| Service | URL |
|---|---|
| Admin portal | http://localhost:3000 |
| API | http://localhost:8090 |
| API health / readiness | http://localhost:8090/healthz · `/readyz` |
| API metrics | http://localhost:9091/metrics |
| Kafka UI | http://localhost:8085 |
| OpenFGA playground | http://localhost:3010/playground |
| OPA viewer (policies, data, live decision test) | http://localhost:3020 |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3001 (admin/admin) |
| Jaeger | http://localhost:16686 |

`docker compose up` applies migrations (`migrate` runs to completion first),
seeds the permission catalog and system roles, creates the bootstrap
administrator, and registers the OpenFGA authorization model — all idempotent,
so restarting is safe.

### ⚠️ Bootstrap administrator credentials

The bootstrap admin is created from environment variables and is **only** meant
for local development and first-run bootstrap:

```env
BOOTSTRAP_ADMIN_EMAIL=sr.alasgarov@gmail.com
BOOTSTRAP_ADMIN_PASSWORD=Anh0@111118
```

* The password is **never** hardcoded in source — the service refuses to start
  without these variables.
* The account is created with `must_change_password=true`; the portal forces a
  password change before anything else is reachable, and the API sets the flag
  on every new user too.
* Seeding is idempotent: on re-runs it verifies the account and its Super Admin
  role, and never overwrites an existing password.
* **In production**, supply these from a secret manager (AWS Secrets Manager,
  Vault, …), use a unique high-entropy password, rotate it immediately after the
  first sign-in, and prefer creating a per-person platform account over sharing
  the bootstrap one. Do not commit `.env`. The dev values above are public in
  this README precisely because they must never be used anywhere real.

---

## Architecture

Hexagonal (ports & adapters) with DDD and Clean Architecture boundaries. The
dependency rule points inwards: the domain knows nothing about PostgreSQL,
Redis, Kafka, OPA, OpenFGA, HTTP or JWT.

```
        ┌───────────────────────── inbound adapters ─────────────────────────┐
        │  httpapi (chi): routing, middleware, DTO mapping, RFC 7807 errors  │
        └────────────────────────────────┬──────────────────────────────────-┘
                                         │ commands / queries
        ┌────────────────────────────────▼──────────────────────────────────┐
        │  application: use cases, transactions, authorization orchestration │
        │               audit + outbox emission, cache invalidation          │
        └────────────────────────────────┬──────────────────────────────────┘
                                         │ pure domain calls
        ┌────────────────────────────────▼──────────────────────────────────┐
        │  domain: aggregates, permission engine, delegation rule, invariants│
        │          (no imports outside the standard library + errors pkg)    │
        └────────────────────────────────┬──────────────────────────────────┘
                                         │ ports (interfaces)
        ┌────────────────────────────────▼──────────────────────────────────┐
        │  outbound adapters: postgres · redis · kafka · openfga · opa       │
        └───────────────────────────────────────────────────────────────────┘
```

Detailed documents live in [`docs/`](docs/):

| Document | Contents |
|---|---|
| [onboarding.md](docs/onboarding.md) | **Start here** — how a new developer should read, understand and extend the system |
| [architecture.md](docs/architecture.md) | Layers, boundaries and ADRs for every significant decision |
| [domain-model.md](docs/domain-model.md) | Aggregates, entities, state machines |
| [authorization.md](docs/authorization.md) | The three authorization layers and how a decision is made |
| [search.md](docs/search.md) | Dynamic search: the condition tree, field-level search authorization, SQL safety |
| [permission-delegation.md](docs/permission-delegation.md) | The delegation rule and its algorithm |
| [authentication.md](docs/authentication.md) | Login, refresh rotation, reuse detection, lockout |
| [api-client-authentication.md](docs/api-client-authentication.md) | Client-credentials flow and secret handling |
| [multi-tenancy.md](docs/multi-tenancy.md) | Tenant isolation and Row-Level Security |
| [events.md](docs/events.md) | Event envelope, catalog and the transactional outbox |
| [security.md](docs/security.md) | Controls, OWASP API Top 10 mapping, production notes |
| [local-development.md](docs/local-development.md) | Day-to-day workflows |
| [sequence-diagrams.md](docs/sequence-diagrams.md) | Mermaid diagrams for the main flows |

---

## Folder structure

One Go module. Every package is exactly one level under `internal/`, named after
what it is.

```
wallet-gateway/
├── cmd/{api,worker,migrate}/    # the three binaries
├── internal/
│   │                            # ── the core: no vendor SDKs, no I/O ──
│   ├── domain/                  # aggregates, permission engine, search model
│   ├── ports/                   # the interfaces the core needs from the world
│   ├── app/                     # use cases, authorizer, outbox worker
│   │                            # ── the outside world: one package per thing ──
│   ├── postgres/                # SQL repositories, RLS transactions
│   ├── httpapi/                 # chi routes, handlers, DTOs, middleware
│   ├── cache/                   # Redis: permissions, idempotency, rate limits
│   ├── kafka/                   # outbox publisher
│   ├── opa/                     # policy decisions
│   ├── openfga/                 # relationship checks + model.fga
│   │                            # ── plumbing ──
│   ├── setup/                   # config parsing + dependency wiring
│   ├── seed/                    # idempotent bootstrap (admin, FGA tuples)
│   ├── auth/                    # JWT + Argon2id
│   ├── apperr/                  # the one error type the API speaks
│   ├── db/                      # connection pool
│   ├── events/                  # event envelope
│   ├── idempotency/             # Idempotency-Key store
│   ├── observability/           # logging, metrics, tracing
│   └── validation/              # UUID, email, slug checks
├── web/                         # React + TypeScript + Vite + MUI
│   ├── src/{api,auth,components,pages}
│   └── e2e/                     # Playwright specs
├── tests/integration/           # black-box API tests (build tag: integration)
├── migrations/                  # reversible SQL migrations
├── deployments/{docker,opa,openfga,monitoring}
├── openapi.yaml                 # the API contract (generates the TS client)
├── docs/
├── docker-compose.yml
└── Makefile
```

**How to follow a request through the code.** Every path is the same four hops:

```
internal/httpapi/router.go     which URL → which handler
internal/httpapi/handlers_*.go decode, then call one use case
internal/app/<thing>_service.go authorize → transaction → audit → emit event
internal/postgres/<thing>_repo.go the SQL
```

---

## Environment variables

`.env.example` is the reference; every variable is documented there. The ones
that matter most:

| Variable | Purpose |
|---|---|
| `DATABASE_URL` | Owner/migration role — used by `migrate` only |
| `DATABASE_APP_URL` | Application role; **RLS is enforced for it** (it does not own the tables) |
| `JWT_SIGNING_KEY` | ≥32 chars; the service refuses to start otherwise |
| `BOOTSTRAP_ADMIN_EMAIL` / `_PASSWORD` | First-run administrator (see the warning above) |
| `ACCESS_TOKEN_TTL` / `REFRESH_TOKEN_TTL` | Token lifetimes (15m / 30d by default) |
| `OPENFGA_STORE_ID` / `_MODEL_ID` | Optional; auto-discovered locally, **pin them in production** |
| `RATE_LIMIT_RPS` | Per-IP request limit (dev uses 300 because everything shares one IP) |
| `LOGIN_RATE_LIMIT_PER_MIN` | Brute-force protection — keep strict |
| `CORS_ALLOWED_ORIGINS` | Strict allow-list; the portal origin must be listed |

---

## Migrations and seeding

```bash
make migrate-up      # apply everything (also runs automatically in compose)
make migrate-down    # roll back one migration
make seed            # bootstrap admin + OpenFGA tuples (idempotent)
```

Migrations are plain reversible SQL in `migrations/`, applied by
`cmd/migrate` under a PostgreSQL advisory lock so concurrent API replicas
cannot race. `0008_seed_catalog` seeds 56 permission definitions and 16 system
roles; the bootstrap admin is seeded by the API at start-up because it needs
Argon2id hashing.

---

## How the pieces fit

### OpenFGA — relationships
Owns the relationship/ownership graph only: platform membership, tenant
membership, role assignment, resource ownership. The model is a single DSL file
(`internal/openfga/model.fga`) that the
service embeds and applies at start-up — there is no hand-maintained JSON copy
to drift. It never stores permission semantics.

### OPA — request policy
Decides each request from the principal, the request context, the effective
permissions and the relationship results. It returns `allow`, a `reason`,
`allowed_fields`, `denied_fields` and row `filters`. Policies live in
`deployments/opa/policies` with tests beside them (`make opa-test`).

### The domain engine — resource and field permissions
Re-evaluates the action permission after OPA agrees (defence in depth), and
computes field-level visibility. `deny` always beats `allow`.

**All three fail closed.** If OPA or OpenFGA is unreachable, the request is
denied with 503 — never allowed.

### Redis — cache, rate limiting, idempotency
Effective permissions are cached under versioned, tenant-aware keys:

```
wallet-gateway:v1:tenant:{tenantId}:v{version}:principal:{principalId}:permissions
```

Changing a role bumps the tenant's version, invalidating every cached principal
at once. If Redis is down, cache reads simply miss and the database answers —
authorization never fails open. The login limiter is the exception that fails
*closed*: if it cannot count, sign-in is refused.

### Kafka + transactional outbox
Business transactions never talk to Kafka. They append an event to
`outbox_events` in the same transaction as the state change; the worker
publishes it with retries, exponential backoff and dead-lettering. If Kafka is
down, nothing is lost — events wait in the outbox.

---

## Authentication flow

```
POST /api/v1/auth/login  →  access token (15m, in memory)
                            refresh token (30d, rotating)
```

Browsers send `X-Auth-Mode: cookie`; the refresh token then travels **only** in
a HttpOnly, SameSite=Strict cookie scoped to `/api/v1/auth`, and never appears
in a response body. Non-browser clients omit the header and get it in the body.

Every refresh rotates the token. Replaying a rotated token is treated as theft:
the entire token family is revoked. Five failed sign-ins lock the account for
15 minutes. See [docs/authentication.md](docs/authentication.md).

## Authorization flow

```
request → authenticate (JWT)
        → resolve effective permissions (Redis → PostgreSQL)
        → OpenFGA: is the principal related to the target?
        → OPA: does policy allow this request?          ┐ both must agree
        → domain engine: re-check action + fields       ┘
        → handler → response with unreadable fields removed
```

Access tokens deliberately carry **no** permission claims — permissions are
resolved per request, so a revocation takes effect immediately.

## Permission delegation flow

The core business rule: **a principal may only grant permissions it holds
itself, at an equal or narrower scope.**

```
compute the actor's effective permissions
  → canonicalise the requested grant (service.resource.action[.field] + scope + effect + conditions)
  → same definition? scope no wider? conditions no looser? no explicit deny?
  → does the actor hold `assign`?  → can the actor manage the target (OpenFGA)?
  → persist → update relationships → invalidate cache → audit + outbox event
```

Violations return `403 PERMISSION_DELEGATION_DENIED` and are recorded in the
audit log — including the attempt itself. See
[docs/permission-delegation.md](docs/permission-delegation.md).

## API client flow

```bash
# 1. Create (the secret is shown exactly once)
POST /api/v1/api-clients  → { client_id, client_secret }

# 2. Exchange for a token
curl -X POST http://localhost:8090/api/v1/oauth/token \
  -H 'Content-Type: application/json' \
  -d '{"grant_type":"client_credentials","client_id":"wg_…","client_secret":"…"}'

# 3. Call the API
curl http://localhost:8090/api/v1/users -H "Authorization: Bearer <access_token>"
```

Secrets are stored as Argon2id hashes and can never be retrieved — only
rotated, which invalidates the previous one immediately.

---

## Running the tests

```bash
make test              # Go unit tests (domain, application, adapters)
make opa-test          # OPA policy tests
make test-integration  # black-box API tests — needs the stack up
make test-e2e          # Playwright — needs the stack up
cd web && npm test   # React/TypeScript unit tests
```

The integration and E2E suites run against a **real** stack:

```bash
docker compose up --build -d
make test-integration
PORTAL_URL=http://localhost:3000 make test-e2e
```

Both suites share a small number of sessions on purpose: sign-in is rate
limited as brute-force protection, and refresh tokens are single-use, so a
suite that logged in per test would trip its own security controls.

---

## Developer experience

```bash
make setup          # env file, Go modules, npm install
make dev            # infra in docker + API on the host
make build          # compile all binaries
make lint           # go vet + eslint
make format         # gofmt + prettier
make openapi        # regenerate the TypeScript client from the OpenAPI spec
make docker-up      # full stack
make docker-down    # stop and remove volumes
```

CI (`.github/workflows/ci.yml`) runs build, lint, unit tests, OPA tests,
migration checks, integration tests, the portal build, E2E, and security and
dependency scans (`govulncheck`, `gosec`, `npm audit`, Trivy).

Pre-commit is worth wiring locally: `gofmt`, `go vet`, `npm run lint`, and a
secret scanner (e.g. `gitleaks`). Never commit `.env`.

---

## Production security notes

* **Secrets** — supply `JWT_SIGNING_KEY`, database passwords and the bootstrap
  credentials from a secret manager. Rotate the signing key with an overlap
  window (accept the old key for one access-token TTL).
* **TLS everywhere** — the refresh cookie sets `Secure` automatically behind an
  HTTPS terminator (`X-Forwarded-Proto: https`).
* **Database roles** — the app must connect as `wallet_app`, never as the table
  owner: Row-Level Security does not apply to owners, so this separation is what
  makes tenant isolation a database guarantee rather than a code convention.
* **Pin OpenFGA** — set `OPENFGA_STORE_ID` and `OPENFGA_MODEL_ID` so deployments
  are deterministic instead of discovering the store at boot.
* **Rate limits** — the dev value of `RATE_LIMIT_RPS` is generous because
  everything shares one source IP locally. Tune per environment; keep
  `LOGIN_RATE_LIMIT_PER_MIN` strict.
* **Kafka** — enable TLS/SASL and disable auto topic creation; the compose file
  is plaintext for local convenience only.
* **Audit retention** — `audit_logs` is append-only (a database trigger blocks
  UPDATE/DELETE, and the app role has no such grants). Plan partitioning and
  archival before volume becomes a problem.
* **Observability** — the API exports Prometheus metrics and OTLP traces. Alert
  on `authz_decisions_total{decision="deny"}` spikes and on outbox lag.
