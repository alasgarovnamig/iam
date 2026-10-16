# Authorization

Three layers, all of which must agree, all of which fail closed.

| Layer | Question | Where |
|---|---|---|
| OpenFGA | Is the principal *related* to the target? | `internal/openfga` + `model.fga` |
| OPA | Does policy allow this *request*, and which fields? | `deployments/opa/policies/authz.rego` |
| Domain engine | Do the *permissions* actually cover this action and these fields? | `domain/permission.go` |

Deny by default: nothing is allowed unless a rule grants it.

## The permission model

```
service . resource . action [ . field ]   +  scope  +  effect  +  conditions
```

* **service** — `user-management` today; future services add their own.
* **resource** — `users`, `roles`, `tenants`, `api-clients`, `audit-logs`, `sessions`, `permissions`.
* **action** — `create`, `read`, `update`, `delete`, `search`, `list`, `assign`, `revoke`, `export`, `rotate`.
* **field** — optional; makes the grant field-level.
* **scope** — `own` · `assigned` · `team` · `organization` · `descendants` · `tenant` · `all` (narrow → wide).
* **effect** — `allow` or `deny`. **Deny always wins**, at any scope that reaches the target.
* **conditions** — key/value constraints the grant must satisfy.

Examples:

```
user-management.users.create
user-management.users.read.email
user-management.users.update.first_name
user-management.users.search.phone
user-management.roles.assign
user-management.api-clients.create
```

## Field-level permissions

An action grant covers base fields (`id`, `tenant_id`, `user_type`, timestamps,
`version`). Every protected field needs its own grant:

* **Read** — fields without `users.read.<field>` are **omitted from the
  response**, not nulled. Secret material (`password_hash`,
  `refresh_token_hash`) has no permission at all and never leaves the database.
* **Update/Create** — a body carrying a field without `users.update.<field>`
  is rejected with `403 FIELD_NOT_ALLOWED`. Unknown fields are `400`.
* **Search** — filtering by a field needs `users.search` **and**
  `users.search.<field>`. Reading a field does not imply searching by it: a
  support agent may see one customer's email without being able to enumerate
  the user base by email.

## Scopes

| Scope | Reaches |
|---|---|
| `own` | Only the principal itself |
| `assigned` | Resources explicitly assigned to the principal |
| `team` | The principal's immediate organization unit |
| `organization` | The principal's organization subtree |
| `descendants` | The subtree below the principal's unit |
| `tenant` | Everything in the principal's tenant |
| `all` | Platform-wide (platform principals only) |

Collection requests (`create`, `list`, `search`) have no concrete target row, so
a tenant-wide grant reaches them inside the principal's own tenant.

## The decision pipeline

```
authenticate (JWT: identity only, no permissions)
  → load effective permissions   (Redis → PostgreSQL; miss ≠ allow)
  → OpenFGA Check                (member of the tenant/platform?)
  → OPA decision                 (tenant boundary, permission match, fields, filters)
  → domain re-check              (defence in depth; a disagreement denies)
  → handler                      (response filtered to allowed fields)
```

The tenant boundary is checked in three independent places: the application
(`You cannot create users in another tenant`), OPA (`cross_tenant_violation`)
and PostgreSQL RLS. Any one of them alone would stop a leak; all three together
mean a single mistake cannot.

## OPA

Input:

```json
{
  "principal": { "id": "…", "type": "tenant_user", "tenant_id": "…" },
  "request":   { "method": "POST", "path": "/api/v1/users", "action": "create", "resource": "users" },
  "target":    { "tenant_id": "…", "resource_id": null },
  "permissions": [{ "code": "user-management.users.create", "scope": "tenant", "effect": "allow" }],
  "relationships": { "is_self": false, "same_tenant": true, "platform": false, "member": true },
  "context": { "ip": "127.0.0.1", "time": "2026-01-01T00:00:00Z" }
}
```

Output:

```json
{
  "allow": true,
  "reason": "permission_granted",
  "allowed_fields": ["email", "first_name"],
  "denied_fields": ["internal_notes"],
  "filters": { "tenant_id": "…" }
}
```

Reasons: `permission_granted`, `permission_missing`, `scope_insufficient`,
`explicit_deny`, `cross_tenant_denied`, `permission_denied`.

Run the policy tests with `make opa-test` (15 cases: deny-by-default, tenant
isolation, deny precedence, scope evaluation, field output, row filters).

## OpenFGA

Owns relationships only. Types: `platform`, `tenant`, `organization_unit`,
`role`, `user`, `api_client`, `resource`. Relations the service depends on by
name: `platform#member`, `platform#super_admin`, `tenant#member`, `role#assignee`.

The model is one DSL file, embedded and applied at start-up (ADR-006).
`resource` already carries `tenant`/`owner`/`manager`/`viewer`, so future
services attach to it without a model rewrite.

## Failure behaviour

| Dependency | Down | Behaviour |
|---|---|---|
| OpenFGA | ✕ | `503`, request denied, denial audited |
| OPA | ✕ | `503`, request denied, denial audited |
| Redis (permission cache) | ✕ | Cache miss → PostgreSQL; **never** an implicit allow |
| Redis (login limiter) | ✕ | Sign-in refused (`503`) — the one place we fail closed on Redis |
| Redis (general limiter) | ✕ | Requests proceed; availability wins for non-auth traffic |
| PostgreSQL | ✕ | `500`/`503` — nothing is authorized without the source of truth |

## Caching

```
wallet-gateway:v1:tenant:{tenantId}:v{version}:principal:{principalId}:permissions   TTL 2m
```

Versioned and tenant-aware. A role change bumps `tenant:{t}:permver`, so every
cached principal in that tenant is invalidated at once — no key scanning. Direct
grants delete just that principal's key.
