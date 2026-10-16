# Architecture

## Goal

One gateway that customer companies integrate with instead of a dozen
providers. Everything here is foundation: the services that will carry the
money (wallets, payments, transactions, provider routing) do not exist yet, so
the job of this repository is to make adding them cheap and safe.

That shapes every decision below. Identity, tenancy, authorization and audit
are the parts you cannot retrofit — a payment service that assumed a weaker
tenant boundary would have to be rewritten, not extended.

## Layering

Hexagonal architecture, with the dependency rule pointing inwards.

```
inbound adapter (httpapi)
        │  DTOs in, DTOs out. No business rules.
        ▼
application (use cases)
        │  Transactions, authorization orchestration, audit, outbox, cache.
        ▼
domain
        │  Aggregates, the permission engine, the delegation rule.
        │  Imports nothing but the standard library and the errors package.
        ▼
ports (interfaces) ◄──── implemented by ──── outbound adapters
                                              postgres · redis · kafka
                                              openfga · opa
```

What lives where, concretely:

| Package | Contains | Never contains |
|---|---|---|
| `internal/domain/` | Permission algebra, delegation rule, state machines, invariants | SQL, HTTP, JWT, vendor SDKs |
| `internal/ports/` | Interfaces the core needs from the world | Implementations |
| `internal/app/` | Use cases, transaction boundaries, authorizer, outbox worker | SQL strings, `http.Request`, chi, pgx |
| `internal/postgres/`, `internal/httpapi/`, `internal/cache/`, `internal/kafka/`, `internal/opa/`, `internal/openfga/` | pgx queries, chi handlers, Redis keys, Kafka writers, OPA/OpenFGA HTTP | Business rules |
| `internal/setup/` | Config parsing, dependency wiring | Business rules |

Everything else under `internal/` is small and self-describing: `auth` (JWT +
Argon2id), `apperr` (the one error type), `db` (pool), `events` (envelope),
`idempotency`, `observability`, `validation`, `seed`.

There is no `adapters/` grouping level. A package that talks to Postgres is
called `postgres`; the grouping added a directory without adding information.

The test that keeps the dependency rule honest: `internal/domain` compiles with
no third-party imports, and its tests need no fakes at all — they construct
permission sets directly.

## Why one flat module

Earlier this repo was laid out as a monorepo (`apps/<service>/internal/...` plus
a shared `packages/go/` library). That shape costs something on every single
read — five directory levels before the first real file, import paths that wrap,
and a "shared library" whose only consumer was the one service importing it.

It is one service, so it is one module. If a second service ever arrives, it
gets its own repository and imports what it needs; splitting a package out later
is a rename, which is cheap. Guessing at reuse before there is a second consumer
is not.

---

# Architecture Decision Records

## ADR-001: Split authorization across OpenFGA, OPA and a domain engine

**Status:** accepted

**Context.** The requirements are three different questions wearing one word.
"Is this user a member of that tenant?" is a graph question. "Is this request
allowed right now, from this IP, on this path?" is a policy question. "Which
fields of this response may they see?" is a data question tied to our own
permission catalog.

**Decision.** Give each question to the tool that answers it well:

* **OpenFGA** owns relationships and ownership only — membership, role
  assignment, resource ownership. Never permission semantics.
* **OPA** owns request-context policy: the tenant boundary, permission matching
  with scope and effect, field-level output, row filters.
* **The domain engine** re-evaluates the action permission and computes field
  visibility in Go.

**Consequences.** Two network calls per authorized request (both cached or
fast, and OPA is a sidecar). In exchange, relationship data does not leak into
policy, policy is testable in isolation (`make opa-test`), and the domain rules
are unit-testable with no infrastructure at all. The domain re-check is
deliberate redundancy: if OPA and the engine ever disagree, the request is
denied.

**Rejected:** putting everything in OpenFGA (it cannot express field-level
grants or conditions cleanly); putting everything in OPA (relationship graphs
in Rego are slow and awkward); rolling our own (this is the part you least want
to hand-roll).

## ADR-002: Permissions are resolved per request, never carried in the JWT

**Status:** accepted

**Context.** Embedding permissions in the token is faster and tempting.

**Decision.** The JWT carries identity only: `sub`, `principal_type`,
`tenant_id`, `session_id`, `client_id`, `jti`, `iss`, `aud`, `iat`, `nbf`,
`exp`. Permissions are resolved per request from Redis, falling back to
PostgreSQL.

**Consequences.** A revoked permission takes effect immediately rather than at
the end of the access-token TTL — for a financial platform, a 15-minute window
where a fired employee still has access is not acceptable. The cost is a lookup
per request, which is why the effective permission set is cached under a
versioned tenant key.

## ADR-003: Tenant isolation is enforced by PostgreSQL, not only by our code

**Status:** accepted

**Context.** Application-level `WHERE tenant_id = ?` is one forgotten clause
away from a cross-tenant leak, and that class of bug is exactly what a
regulator asks about.

**Decision.** Row-Level Security on every tenant-scoped table. The application
connects as `wallet_app`, which does **not** own the tables, so RLS applies to
it. Each unit of work sets `app.tenant_id` (or `app.is_platform`) with
`SET LOCAL` inside the transaction. With no GUC set, nothing is visible: deny
by default.

**Consequences.** Every repository call must run inside `TxManager.WithTx` —
the repositories refuse to run otherwise, since a query outside a transaction
would carry no RLS context. Migrations keep using the owner role. The payoff is
that a missing filter in Go is a bug, not a breach.

**Verified:** with the tenant GUC set to tenant A, selecting tenant B's user by
its exact id returns zero rows.

## ADR-004: Authorization fails closed, caching fails soft

**Status:** accepted

**Decision.** If OPA or OpenFGA is unreachable, deny with `503`. If Redis is
unreachable, treat cache reads as misses and answer from PostgreSQL — except
the login limiter, which refuses sign-in when it cannot count.

**Consequences.** An OPA outage is an availability incident, not a security
one. A Redis outage costs latency, not correctness. The one place Redis failure
is fatal is brute-force protection, where failing open would be worse than
failing.

## ADR-005: Business rejections must commit their side effects

**Status:** accepted

**Context.** The natural Go shape — return an error from inside the transaction
closure — silently rolled back the very evidence a rejection produces. Failed
login counters never persisted (so accounts never locked), refresh-token family
revocation was undone (so a stolen token chain stayed usable), and
privilege-escalation attempts vanished from the audit log. All three looked
correct in code review and all three were verified broken end-to-end.

**Decision.** A business rejection travels out of the closure in an `outcome`
variable and is returned after the commit; only infrastructure errors are
returned from the closure. Denials that follow a rollback are audited in their
own transaction (`auditDelegationFailure`).

**Consequences.** A slightly unusual shape, so it is commented at each site and
pinned by tests whose fake transaction manager rolls back on closure errors —
reintroducing the pattern fails the suite loudly.

## ADR-006: The OpenFGA model is a DSL file the service applies at boot

**Status:** accepted

**Context.** The alternatives were a hand-maintained JSON copy (drifts from the
DSL) or a CLI step in compose (a store id that has to reach the API somehow).

**Decision.** `model.fga` lives in the OpenFGA adapter, is embedded with
`go:embed`, and is transformed and applied at start-up. The store is found by
name or created. `OPENFGA_STORE_ID`/`OPENFGA_MODEL_ID` override discovery.

**Consequences.** One source of truth, and a boot that self-heals on a fresh
OpenFGA. To avoid a new model version on every restart, the stored model is
compared to ours by rendering both back to DSL — a byte comparison fails
because OpenFGA normalizes what it stores. Production should pin both ids.

## ADR-007: Refresh tokens rotate; reuse revokes the family

**Status:** accepted

**Decision.** Refresh tokens are opaque, stored as SHA-256 hashes, single-use,
and rotate on every refresh. Presenting a rotated token revokes the whole
family.

**Consequences.** A stolen token is usable at most once, and the theft is
detected the moment either party uses it again. The trade-off is real and shows
up in tests: any client that replays a token — including a naive test suite
reusing saved cookies — trips detection. That is the control working.

## ADR-008: Browsers never see the refresh token

**Status:** accepted

**Decision.** Clients opt in with `X-Auth-Mode: cookie`; the refresh token then
travels only in a HttpOnly, SameSite=Strict cookie scoped to `/api/v1/auth`,
and is omitted from the response body. The access token lives in memory only.
Non-browser clients omit the header and get the token in the body.

**Consequences.** XSS cannot read the refresh token, and SameSite=Strict covers
the CSRF surface of the refresh endpoint. Nothing is in `localStorage`. A page
reload restores the session by calling `/auth/refresh` with the cookie. Custom
headers require the API's CORS allow-list to include `X-Auth-Mode` — it does.

## ADR-009: Field permissions are explicit grants, not implied by the action

**Status:** accepted

**Decision.** `users.read` grants base fields; every protected field needs
`users.read.<field>`. Search is separate again: `users.search.email` is
required to filter by email even if `users.read.email` is held. Unreadable
fields are omitted from responses; unwritable fields in a request body reject
the whole request.

**Consequences.** More catalog entries (56), and a support agent who can see a
customer's name genuinely cannot enumerate the user base by email — reading and
searching are different capabilities with different risk.

## ADR-010: Transactional outbox for every event

**Status:** accepted

**Decision.** Business transactions append to `outbox_events`; a worker
publishes to Kafka with exponential backoff and dead-lettering.

**Consequences.** No event is published for a transaction that rolled back, and
no state change is lost because Kafka was down. Delivery is at-least-once, so
consumers must be idempotent — the envelope carries `event_id` for exactly that.

## ADR-011: Optimistic locking on mutable aggregates

**Status:** accepted

**Decision.** `version` on tenants, users, roles and API clients. Updates carry
the expected version; a mismatch returns `409 VERSION_CONFLICT`.

**Consequences.** Concurrent edits surface as a conflict the user can resolve
instead of a silent last-write-wins — which, on a permission set, would mean
silently restoring access someone had just removed.

## ADR-012: chi for routing

**Status:** accepted

**Decision.** `go-chi/chi` in the HTTP adapter.

**Consequences.** Standard `http.Handler` semantics, so middleware is ordinary
Go and the framework never reaches past the adapter. Swapping it would touch
`internal/httpapi` only.

## ADR-013: Argon2id for passwords and client secrets

**Status:** accepted

**Decision.** Argon2id (64 MB, 3 iterations, parallelism 2) with a PHC-encoded
hash; comparison is constant-time. Client secrets are hashed the same way and
shown exactly once.

**Consequences.** Login costs ~50 ms of CPU by design. Tests use cheap
parameters explicitly rather than weakening the default.
