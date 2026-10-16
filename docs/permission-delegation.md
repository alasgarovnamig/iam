# Permission delegation

**A principal may only grant permissions it holds itself, at an equal or
narrower scope.** This is the rule that makes self-service administration safe:
tenant admins manage their own people without ever being able to manufacture
authority they were not given.

Enforced in `internal/domain/permission.go`
(`PermissionSet.CanDelegate`), inside a transaction, on every path that hands
out authority — role permissions, direct user grants, API client grants, and
role assignment (a role is a bundle of permissions, so assigning it delegates
all of them).

The portal mirrors the rule to shape the UI. That is guidance, not enforcement:
the API re-runs the check and is the authority.

## What "subset" means

A grant is delegable when **all** of these hold against at least one permission
the actor holds:

| Dimension | Rule |
|---|---|
| Definition | Identical `service.resource.action[.field]`. An action grant does **not** imply its fields, and a field grant does **not** imply the action. |
| Scope | The requested scope is no wider: `own/assigned < team < organization < descendants < tenant < all`. |
| Conditions | Every condition binding the actor must also bind the grant. The grant may add more. |
| Deny | An explicit deny on the definition blocks delegating it entirely. |
| Effect | Granting a `deny` requires holding the matching `allow` — you cannot deny what you have no authority over. |

Two hard boundaries sit alongside the subset check:

* Platform-only definitions can never reach a tenant role, tenant user or API
  client — even if the actor somehow held them.
* Tenant principals can never receive `scope=all`.

## Examples

| Actor holds | Tries to grant | Result |
|---|---|---|
| `users.create` @ tenant | `users.create` @ tenant | ✅ same |
| `users.create` @ tenant | `users.create` @ own | ✅ narrower |
| `users.create` @ tenant | `users.delete` @ tenant | ❌ not held |
| `users.read` @ own | `users.read` @ tenant | ❌ wider scope |
| `users.read` @ tenant | `users.read` @ all | ❌ wider scope |
| `users.read.email` | `users.read.phone` | ❌ different field |
| `users.read` (action) | `users.read.email` | ❌ the action does not imply the field |
| `users.read.email` | `users.read` (action) | ❌ the field does not imply the action |
| `users.read` @ tenant + deny | `users.read` @ own | ❌ explicitly denied |
| `users.read` {dept: support} | `users.read` (no conditions) | ❌ conditions dropped |
| `users.read` {dept: support} | `users.read` {dept: support, region: az} | ✅ tighter |
| Support Agent role | Super Admin permissions | ❌ not held |

## The algorithm

```
1.  Compute the actor's effective permissions (roles ∪ direct grants), cached.
2.  Canonicalise the requested grant:
        service.resource.action[.field] + scope + effect + conditions
3.  Check the actor holds `permissions.assign` (or `roles.assign`, or
    `api-clients.assign`) for the target's tenant — authority to delegate at all.
4.  Check the relationship: can the actor manage this target? (OpenFGA; the
    tenant boundary is absolute for tenant principals.)
5.  Reject platform-only definitions on tenant subjects, and scope=all for
    tenant subjects.
6.  Apply the subset rule above (CanDelegate / CanDelegateAll).
7.  Persist inside the transaction, together with the audit record and the
    outbox event.
8.  Update OpenFGA relationships (role assignment).
9.  Invalidate the cache: bump the tenant permission version for role changes,
    or drop the principal's key for direct grants.
10. On rejection: 403 PERMISSION_DELEGATION_DENIED, and audit the attempt in a
    separate transaction — the business transaction has rolled back, and losing
    the record of an escalation attempt is not acceptable (see ADR-005).
```

Batch grants (role assignment) are all-or-nothing: `CanDelegateAll` rejects the
whole bundle if any single permission is not delegable, so no one is left with
a half-applied role.

## Why the audit is a separate transaction

A rejection is reported as an error, and an error rolls the transaction back —
including any audit row written inside it. Privilege-escalation attempts are
exactly what security review asks for, so denials are recorded through
`auditDelegationFailure`, which opens its own transaction after the rollback.

## Where it is tested

| Test | Covers |
|---|---|
| `internal/domain/permission_test.go` | 18 table-driven cases: same/narrower/wider scope, field vs action, deny precedence, conditions, batch rejection, deny delegation |
| `tests/integration/authz_integration_test.go` | The rule over real HTTP: held permission ✅, platform-only ❌, wider scope ❌, and the denial audit |
| `web/src/auth/permissions.test.ts` | The UI mirror agrees with the backend |
| `web/e2e/admin-flows.spec.ts` | Granting through the permission tree |

## The UI

`PermissionTree` renders service → resource → action → field. Entries the actor
cannot delegate are disabled with a lock icon and the reason ("Your own scope
for this permission is 'own'; you cannot grant a wider scope"). The scope picker
only offers scopes the actor can actually delegate; wider ones are visible but
disabled and labelled, so the boundary is legible rather than mysterious.
