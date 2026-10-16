# Authentication

Two flows: interactive users and machine clients. Both issue JWT access tokens
that carry identity only — permissions are resolved per request (ADR-002).

## Access tokens

| Claim | Meaning |
|---|---|
| `sub` | Principal id (user id, or API client id) |
| `principal_type` | `platform_user` · `tenant_user` · `api_client` · `service_account` · `system_service` |
| `tenant_id` | Empty for platform principals |
| `session_id` | User sessions only |
| `client_id` | API clients only |
| `jti`, `iss`, `aud`, `iat`, `nbf`, `exp` | Standard claims; all validated |

HS256, TTL 15 minutes (30 for clients). Verification enforces the algorithm,
issuer, audience and time claims — an `alg: none` or RS/HS confusion token is
rejected.

## User sign-in

```
POST /api/v1/auth/login { email, password, tenant_slug? }
```

`tenant_slug` is only needed when the same email exists in several tenants; a
platform account wins otherwise.

* **Rate limited** — 10/min per IP, and **fails closed**: if Redis cannot
  count, sign-in is refused rather than left unprotected.
* **Argon2id** verification (64 MB, t=3, p=2), constant-time.
* **Lockout** — 5 consecutive failures lock the account for 15 minutes and set
  `status = locked`. The correct password then still returns `403
  ACCOUNT_LOCKED`.
* **Uniform errors** — unknown email and wrong password are indistinguishable
  (`INVALID_CREDENTIALS`, "Email or password is incorrect.").
* **Audited** — success, failure and denial, with the reason.

The failed-attempt counter, the lock and the audit row are **committed even
though the request is rejected**; see ADR-005 for why that needs saying.

## Refresh rotation and reuse detection

Refresh tokens are opaque (32 random bytes), stored only as SHA-256 hashes,
single-use, and belong to a *family* created at sign-in.

```
refresh(token):
  session = find by hash                      → unknown? 401
  if session.status != active:                # already rotated, or revoked
      revoke the ENTIRE family                # committed
      audit(denied, refresh_token_reuse_detected)
      return 401 TOKEN_REUSE_DETECTED
  if expired: 401
  session.status = rotated
  issue a new session in the same family      # new refresh token
```

So a stolen token works at most once, and the moment either the thief or the
victim uses it again, both are locked out and the event is recorded. This is
also why a test suite that replays a saved cookie trips detection — the control
working, not a bug.

## Browser token handling

Browsers send `X-Auth-Mode: cookie`:

* the refresh token is returned **only** as `wg_refresh_token` — `HttpOnly`,
  `SameSite=Strict`, `Path=/api/v1/auth`, `Secure` behind TLS — and never
  appears in a response body;
* the access token lives in a JavaScript variable, never in `localStorage` or
  `sessionStorage`;
* a page reload restores the session by calling `/auth/refresh` with the cookie;
* a 401 triggers a single-flight refresh and one replay of the original request.

XSS therefore cannot read the refresh token, and `SameSite=Strict` covers the
CSRF surface of the refresh endpoint. Non-browser clients omit the header and
receive the token in the body.

## Password policy and change

At least 10 characters with upper case, lower case, a digit and a symbol;
128 max. Enforced server-side (`internal/auth`) and mirrored in the portal
with the same rules.

`POST /auth/change-password` verifies the current password, rotates the hash,
clears `must_change_password`, and revokes **every other session** — a password
change is how you evict someone.

Every user, including the bootstrap admin, starts with
`must_change_password=true`; the portal blocks the whole app until it is done.

## Sessions

`GET /auth/sessions` lists your active sessions (device, IP, first and last
use, which one is current). `DELETE /auth/sessions/{id}` revokes one — your own
only. `POST /auth/logout-all` revokes all.

Sessions are also revoked automatically on password change, on suspend/disable,
on delete, and on refresh-token reuse.

## Password reset (extension point)

Not implemented. The structure it needs is already here: `Session` families,
`Credential` with `password_changed_at`, audit actions and outbox events. A
reset service adds a single-use token table, reuses `AuthService.ChangePassword`
for the final step, and emits `user.password_reset.v1`. No schema change to
existing tables is required.
