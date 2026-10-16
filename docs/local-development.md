# Local development

## Prerequisites

Docker Desktop (or Docker Engine + compose v2), Go 1.25+, Node 20+.

## First run

```bash
cp .env.example .env
docker compose up --build          # ~3–5 minutes the first time
```

Portal http://localhost:3000 · API http://localhost:8090. Sign in with the
bootstrap admin from `.env`; you will be forced to change the password.

## Day to day

```bash
# Infra in docker, API on the host (fast rebuilds, debugger-friendly)
docker compose up -d postgres redis kafka opa openfga-db openfga-migrate openfga
make migrate-up
make dev

# Portal with hot reload against that API
cd web && npm run dev      # http://localhost:5173
```

`http://localhost:5173` is already in the CORS allow-list.

Running the API on the host needs the app-role URL, since compose's value points
at the `postgres` hostname:

```bash
export DATABASE_APP_URL="postgres://wallet_app:wallet_app_dev_password@localhost:5432/wallet_gateway?sslmode=disable"
```

## Tests

```bash
make test               # Go unit
make opa-test           # Rego policies
make test-integration   # black-box API — needs the stack up
make test-e2e           # Playwright — needs the stack up
cd web && npm test
```

Both integration suites talk to a real stack:

```bash
docker compose up --build -d
make test-integration
PORTAL_URL=http://localhost:3000 make test-e2e
```

They share a handful of sessions deliberately — login is rate limited and
refresh tokens are single-use, so a suite that authenticated per test would trip
its own security controls.

## Useful commands

```bash
docker compose ps                                   # what is healthy
docker compose logs -f api                          # follow the API
docker compose exec postgres psql -U wallet -d wallet_gateway
docker compose exec redis redis-cli KEYS 'wallet-gateway:*'
docker compose exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list
curl -s localhost:8090/readyz | jq                  # dependency health
```

Inspect the permission cache and its invalidation:

```bash
docker compose exec redis redis-cli GET "wallet-gateway:v1:tenant:<id>:permver"
docker compose exec redis redis-cli KEYS 'wallet-gateway:*:permissions'
```

Watch the outbox drain:

```bash
docker compose exec postgres psql -U wallet -d wallet_gateway \
  -c "SELECT status, count(*) FROM outbox_events GROUP BY 1;"
```

## Changing things

**A new API endpoint** — domain rule → application use case (authorize, audit,
emit) → port if a new dependency is needed → adapter → handler → route →
OpenAPI → `make openapi` → tests.

**A new permission** — add it to `migrations/0008_seed_catalog.up.sql` (or a new
migration), attach it to the system roles that should hold it, and add the code
to `P` in the portal's `auth/permissions.ts`.

**The OpenFGA model** — edit `internal/openfga/model.fga` only; it is
embedded and applied at boot. `go test ./...` validates it transforms.

**An OPA policy** — edit `deployments/opa/policies/authz.rego`, add a test in
`deployments/opa/tests/`, run `make opa-test`. OPA reloads the mounted files, so
no rebuild is needed.

## Reset

```bash
docker compose down -v      # drops all volumes: databases, Kafka, Grafana
docker compose up --build
```

## Troubleshooting

| Symptom | Cause |
|---|---|
| `429` during a test run | The rate limiter. Locally everything shares one IP; raise `RATE_LIMIT_RPS` in `.env`, keep `LOGIN_RATE_LIMIT_PER_MIN` strict. |
| `TOKEN_REUSE_DETECTED` in a client | Something replayed a rotated refresh token. Working as designed — do not "fix" it by disabling rotation. |
| `403 ACCOUNT_LOCKED` | Five bad passwords. `POST /users/{id}/unlock`, or wait 15 minutes. |
| Login blocked by CORS | The portal origin is missing from `CORS_ALLOWED_ORIGINS`, or a custom header is not in the allow-list. |
| `no transaction in context` | A repository was called outside `TxManager.WithTx` — it would have no RLS context. |
| Empty query results as `wallet_app` | No tenant GUC set. That is deny-by-default RLS doing its job. |
| API exits with `openfga not ready` | OpenFGA has not finished booting; compose retries, or start it first. |
