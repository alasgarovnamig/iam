# Dynamic search

A resource can be searched with a **dynamic condition tree** — fields combined
with AND/OR and comparison operators — instead of a fixed set of query
parameters. The same engine backs `users`, `tenants` and `api-clients`.

Two authorization stages guard every search:

1. **Action level** — may the actor call search on this resource at all
   (`<resource>.search`)?
2. **Field level** — for **every** field referenced in the filter or sort, does
   the actor hold `<resource>.search.<field>`? A missing grant is rejected by
   name (`403 FIELD_NOT_ALLOWED`, e.g. `search:status`).

Reading a field and searching by it are different capabilities: a support agent
may see one customer's status without being allowed to enumerate the whole
tenant by status.

## The request

```http
POST /api/v1/users/search
POST /api/v1/tenants/search
POST /api/v1/api-clients/search
```

```jsonc
{
  "filter": {
    "op": "and",                       // group: "and" | "or"
    "conditions": [
      { "field": "first_name", "op": "eq",   "value": "Elam" },     // leaf
      { "op": "or", "conditions": [
          { "field": "last_name", "op": "ilike", "value": "swn%" },
          { "field": "status",    "op": "in",    "value": ["active","pending"] }
      ]}
    ]
  },
  "sort": [{ "field": "created_at", "dir": "desc" }],
  "page": 1,
  "page_size": 20
}
```

A node with a `field` is a **leaf** (its `op` is a comparison); otherwise it is a
**group** (its `op` is `and`/`or` with `conditions`). Groups may nest up to
`MaxFilterDepth` (6) levels. The response is the standard list envelope, and
result rows are still **read-field filtered** — you can filter by a field you
cannot read, but you never see a field you lack read permission for.

## Operators by field type

| Field type | Operators |
|---|---|
| text (`email`, `name`, `slug`, …) | `eq`, `neq`, `like`, `ilike`, `in` |
| enum (`status`) | `eq`, `neq`, `in` |
| timestamp (`created_at`) | `eq`, `gt`, `gte`, `lt`, `lte` |

`created_at` is a base field: filtering or sorting by it needs only the
action-level `search` permission, not a per-field grant.

## Searchable fields per resource

| Resource | Fields |
|---|---|
| users | email, first_name, last_name, phone, status, created_at |
| tenants | name, slug, status, created_at |
| api-clients | name, client_id, status, created_at |

The field set is a single source of truth in `domain/search.go`
(`UsersSearchSchema`, `TenantsSearchSchema`, `APIClientsSearchSchema`). It
constrains both the permission check and the SQL builder.

## How it stays safe

Even though the query shape is fully dynamic, there is no SQL-injection surface:

* **Column names** come only from the resource schema allow-list — never from
  the request body.
* **Operators** map through a fixed table.
* **Every value** is a bound parameter (`$N`); `in` uses `col = ANY($n)` with a
  single array parameter.
* The nesting depth is capped, and unknown fields / illegal operators / bad
  value shapes are rejected with `400` before anything reaches the database.

A value like `x'; DROP TABLE users; --` is treated as data: it becomes a bound
parameter, matches nothing, and the table is untouched (covered by
`internal/postgres/search_test.go`).

## Where it lives

| Layer | File | Responsibility |
|---|---|---|
| Domain | `internal/domain/search.go` | Tree model, schemas, validation, field collection |
| Adapter | `internal/postgres/search.go` | Parameterised WHERE + safe ORDER BY builder |
| Application | `internal/app/search.go` | Two-stage authz; per-service `Search` use cases |
| HTTP | `internal/httpapi/handlers_search.go` | Request DTO → command; the three endpoints |
| Frontend | `web/src/components/SearchBuilder.tsx` | Permission-aware query builder |

## Tests

| Test | Covers |
|---|---|
| `domain/search_test.go` | Validation: unknown field, illegal operator, enum values, depth limit, `Fields()` |
| `internal/postgres/search_test.go` | Parameterisation, injection payload stays data, timestamp cast, ORDER BY allow-list |
| `tests/integration` script | Live: AND/OR/LIKE/IN, field-level 403 by name, injection payload harmless |
| `admin-portal/src/api/search.test.ts` | UI value transforms (`contains` → `%v%`), request assembly |

## The UI

`SearchBuilder` renders a flat list of conditions joined by one AND/OR toggle —
enough for "(name AND slug)", "(name OR slug contains …)". It only offers fields
the actor may search (`searchableFields()` filters by `search.<field>`), so an
unsearchable field cannot even be selected. Friendly operators map to the wire
operators (`contains` → `ilike %v%`, `starts with` → `ilike v%`). The backend
re-enforces everything regardless — the UI is guidance, not the boundary.
