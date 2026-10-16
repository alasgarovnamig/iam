# Multi-tenancy

## The parties

| Party | Meaning |
|---|---|
| **Platform** | Us. Platform users have `tenant_id = NULL` and can work across tenants. |
| **Tenant / partner** | A customer company. Its users, API clients, roles, permissions, audit logs, organization units and data all live inside it. |

Every tenant-scoped table carries `tenant_id`. A tenant cannot see another
tenant's data — not by listing, not by guessing an id, not by a forgotten
`WHERE` clause.

## Three independent boundaries

| Layer | Enforcement |
|---|---|
| Application | `dbCtxFor(principal)`, explicit tenant checks, `You cannot create users in another tenant` |
| OPA | `cross_tenant_violation` when `target.tenant_id != principal.tenant_id` for non-platform principals |
| PostgreSQL | Row-Level Security policies on every tenant-scoped table |

Defence in depth is the point: any one layer would stop a leak; all three mean
one mistake cannot cause one.

## Row-Level Security

The application connects as `wallet_app`, which **does not own the tables** —
RLS does not apply to table owners, so this separation is what makes isolation a
database guarantee. Migrations keep using the owner role.

Each unit of work sets the context inside the transaction:

```sql
-- tenant principals
SELECT set_config('app.tenant_id', '<uuid>', true);
-- platform principals and workers
SELECT set_config('app.is_platform', 'true', true);
```

Policies read those GUCs:

```sql
CREATE POLICY users_tenant ON users
    USING (tenant_id = app_tenant_id())
    WITH CHECK (tenant_id = app_tenant_id());

CREATE POLICY users_platform_all ON users
    USING (app_is_platform()) WITH CHECK (app_is_platform());
```

With no GUC set, **nothing is visible**. Deny by default at the storage layer.

Because `SET LOCAL` only exists inside a transaction, repositories refuse to run
outside `TxManager.WithTx` — a query without a transaction would carry no
tenant context, which is exactly the bug RLS exists to prevent.

Verified behaviour:

| Context | `SELECT count(*) FROM users` |
|---|---|
| none | 0 |
| `app.tenant_id = A` | only A's users |
| `app.tenant_id = A`, selecting B's user **by its exact id** | 0 |
| `app.is_platform = true` | all |

## System roles across tenants

System roles live at `tenant_id IS NULL` and are readable by every tenant
(`roles_system_read`), but writable by nobody: `chk_system_roles_global` plus
`is_system = false` guards in every update path. Tenants create their own custom
roles instead.

## Login across tenants

Login is the one place without a tenant identity yet, so it runs in platform
context. The resolver prefers a platform account; if an email exists in several
tenants, the caller must supply `tenant_slug`.

## Cache and events

Cache keys are tenant-scoped and versioned:
`wallet-gateway:v1:tenant:{tenantId}:v{n}:principal:{p}:permissions`. Kafka
events carry `tenant_id` in the envelope, and the outbox row keeps it too, so
downstream consumers can partition and filter by tenant.

## Organization units

`organization_units` gives each tenant a tree for `team`, `organization` and
`descendants` scopes. The table, the FGA type and the scope semantics exist; the
current API does not yet expose CRUD for units — that is the next increment,
and no schema change is needed for it.
