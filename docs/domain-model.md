# Domain model

Pure Go in `internal/domain` — no SQL, HTTP, JWT or
vendor SDKs. Everything below is unit-testable without infrastructure.

## Aggregates and entities

| Type | Role |
|---|---|
| **Platform** | Implicit singleton (`platform:platform` in OpenFGA); platform users have `tenant_id = NULL` |
| **Tenant** | Aggregate root for a customer company |
| **OrganizationUnit** | Tree inside a tenant; backs `team`/`organization`/`descendants` scopes |
| **User** | Aggregate root; `platform_user`, `tenant_user` or `service_account` |
| **Credential** | Password hash and lockout state, separate from `User` so secrets never ride along in a DTO |
| **Session** | A refresh-token family member |
| **Role** | System (global, immutable) or tenant custom |
| **PermissionDefinition** | Catalog entry: service/resource/action/field |
| **RolePermission** | Definition + scope + effect + conditions, attached to a role |
| **UserRole** | Role assignment |
| **UserPermission** | Direct grant, optionally expiring |
| **APIClient** | Machine principal with a hashed secret |
| **APIClientPermission** | Grant to a client |
| **AuditLog** | Immutable security record |
| **OutboxEvent** | Event awaiting publication |

Common fields on mutable entities: `id` (UUID), `created_at`, `updated_at`,
`created_by`, `updated_by`, `version` (optimistic locking).

## Value objects

**Permission** — `service.resource.action[.field]` + `scope` + `effect` +
`conditions`, with `Code()` rendering the canonical string. **PermissionSet**
answers `Allows`, `AllowsField`, `AllowedFields`, `MaxScopeFor` and
`CanDelegate` — the delegation rule lives here.

**Principal** — the authenticated caller: id, kind, tenant, session/client id.

## State machines

```
User:    pending ──▶ active ──▶ suspended ──▶ active
                 └──▶ disabled ◀── (any)
         active ──▶ locked ──▶ active            (5 failed logins → locked)
         disabled is terminal

Tenant:  pending ──▶ active ──▶ suspended ──▶ active
                 └──▶ terminated ◀── (any)       (terminal)

APIClient: active ⇄ suspended
           active ──▶ expired | revoked          (terminal)

Session: active ──▶ rotated ──▶ reused           (theft detected)
                 └──▶ revoked
```

Transitions go through `TransitionTo`, which rejects anything not on the map
with `409 CONFLICT` — an invalid transition is a conflict, not a validation
error.

## Soft delete

Applied to `tenants`, `users`, `roles`, `api_clients` (`deleted_at`), because
audit trails must still resolve who did what. Unique indexes are partial
(`WHERE deleted_at IS NULL`), so a deleted user's email can be reused.

Security-sensitive state is **not** left dangling behind a soft delete:
deleting a user revokes every session and moves the status to `disabled`;
revoking an API client sets `revoked` so no token can be issued, regardless of
the row still existing.

`audit_logs` and `outbox_events` are never deleted by the application at all.

## Field classification (users)

| Class | Fields | Visibility |
|---|---|---|
| Base | `id`, `tenant_id`, `user_type`, `must_change_password`, timestamps, `version` | With the action permission |
| Protected | `email`, `first_name`, `last_name`, `phone`, `status`, `internal_notes`, `security_flags` | Needs `users.read.<field>` |
| Secret | `password_hash`, `refresh_token_hash` | No permission exists — never leaves the database |

## Invariants

* A platform user has no tenant; a tenant user must have one (DB check constraint).
* System roles are global and immutable (`chk_system_roles_global` + guards in every write path).
* Platform roles go to platform users; tenant roles to tenant users.
* Deny beats allow, at any scope that reaches the target.
* You cannot delegate what you do not hold (see permission-delegation.md).
* Audit logs are append-only — enforced by a database trigger, not convention.
