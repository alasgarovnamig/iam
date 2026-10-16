# API client authentication

Machine-to-machine access via the OAuth 2.0 client-credentials grant. API
clients belong to a tenant and never act outside it.

## Lifecycle

```
create → { client_id, client_secret }   ← the secret is shown exactly once
      → POST /oauth/token                → access token (30 min)
      → call the API with Bearer <token>
      → rotate-secret (old one dies immediately)
      → revoke (no further tokens)
```

## Creation

`POST /api/v1/api-clients` returns:

```json
{
  "data": {
    "client": { "client_id": "wg_OktfDAisD8acnhGc", "status": "active", "...": "..." },
    "client_secret": "k1nxNRqQ1_…",
    "warning": "Store this secret now. It cannot be retrieved again."
  }
}
```

The secret is 32 random bytes, stored as an Argon2id hash. It is **not**
retrievable afterwards: `GET /api-clients/{id}` never contains it, and the
database holds only `$argon2id$v=19$m=65536,...`. Lost secrets are rotated, not
recovered. The portal shows it in a dialog that requires an explicit "I have
stored this secret" acknowledgement before closing.

## Token request

```bash
curl -X POST http://localhost:8090/api/v1/oauth/token \
  -H 'Content-Type: application/json' \
  -d '{"grant_type":"client_credentials","client_id":"wg_…","client_secret":"…"}'
```

`application/x-www-form-urlencoded` works too (RFC 6749 style). The response is
the bare OAuth shape, not the platform envelope:

```json
{ "access_token": "eyJ…", "token_type": "Bearer", "expires_in": 1799, "scope": "wallet.read" }
```

The token carries `principal_type=api_client` and `client_id`, and has **no**
`session_id` — that is how user tokens and client tokens are told apart
everywhere downstream.

Checks before a token is issued: status is `active` (not suspended, expired or
revoked), not past `expires_at`, source IP in `allowed_ips` when that list is
non-empty, secret verifies in constant time, and the owning tenant is
operational. Every outcome is audited. Failures return a uniform 401 so clients
cannot be enumerated.

## Authorization

Clients hold permissions of their own, granted under the same delegation rule as
users — you cannot give a client authority you lack. Platform-only permissions
and `scope=all` are refused outright. A client with no grants authenticates
successfully and can call nothing.

## Rotation and revocation

`POST /api-clients/{id}/rotate-secret` issues a new secret and invalidates the
previous one on the spot (verified in the E2E suite: old → 401, new → 200).
`DELETE /api-clients/{id}` revokes the client; token requests then fail.

## Operational notes

* Store the secret in your own secret manager, never in source.
* Rotate on a schedule and whenever someone with access leaves.
* Use `allowed_ips` when the caller has stable egress addresses.
* Grant the narrowest scope that works — `own`/`assigned` over `tenant`.
* `last_used_at` reveals clients that are no longer used and should be revoked.
