# Security

## Controls

| Control | Implementation |
|---|---|
| Deny by default | No permission → no access. RLS with no context → no rows. OPA `default allow := false`. |
| Least privilege | Field-level grants; scopes; the delegation rule caps what anyone can hand out. |
| Tenant isolation | Application + OPA + PostgreSQL RLS (three independent layers). |
| No plaintext passwords | Argon2id (64 MB, t=3, p=2), PHC-encoded. |
| No plaintext refresh tokens | SHA-256 hashes only; the token is opaque random bytes. |
| No plaintext API secrets | Argon2id; shown exactly once, then unrecoverable. |
| Constant-time comparison | `subtle.ConstantTimeCompare` in the Argon2id verifier. |
| Secure random | `crypto/rand` for tokens, secrets and client ids. |
| Password policy | ≥10 chars, upper + lower + digit + symbol, ≤128. |
| Brute-force protection | 10 sign-ins/min per IP (**fails closed**); 5 failures → 15-minute lock. |
| Rate limiting | Per-IP request limiter; separate stricter limiter for login and OAuth. |
| Account locking | `status = locked` + `locked_until`; the correct password still returns 403. |
| Refresh rotation | Single-use tokens, rotated on every refresh. |
| Reuse detection | Replaying a rotated token revokes the entire family. |
| Secret rotation | API client secrets rotate; the old one dies immediately. |
| Sensitive field redaction | `RedactSensitive` on audit states; secrets have no read permission at all. |
| SQL injection | Parameterised queries throughout; sort/filter fields are allow-listed, never interpolated. |
| Mass assignment | Unknown body fields → 400; protected fields need `update.<field>` → 403. |
| Explicit DTO mapping | Hand-written DTOs; no struct auto-binding into entities. |
| CORS | Strict origin allow-list; credentials enabled only for listed origins. |
| Security headers | `nosniff`, `DENY`, `no-referrer`, `no-store`, restrictive CSP. |
| Request size limits | `MaxBytesReader` (1 MB default). |
| Timeouts | Per-request context timeout; server read/write/idle timeouts. |
| Input validation | Domain + `internal/validation`; UUID, email and slug checks. |
| Audit logging | Every security-sensitive action, including denials and escalation attempts. |
| No secrets in logs | Structured logs carry ids and reasons, never credentials. |
| Privilege escalation prevention | The delegation rule, platform-only guards, `scope=all` guards, immutable system roles. |

## OWASP API Security Top 10

| Risk | How it is addressed |
|---|---|
| **API1 Broken object level authorization** | Every object is authorized against its own tenant/owner, plus RLS. Reading another tenant's user by exact id returns 404 — verified. |
| **API2 Broken authentication** | Argon2id, lockout, rate limiting, rotating single-use refresh tokens with reuse detection, HttpOnly cookies, full JWT claim validation. |
| **API3 Broken object property level authorization** | Field-level read/update/search permissions. Unreadable fields are omitted; unwritable fields reject the request; unknown fields are rejected. |
| **API4 Unrestricted resource consumption** | Rate limits, `page_size` capped at 100, body size limit, request timeouts, connection pool bounds. |
| **API5 Broken function level authorization** | Permissions are `resource.action`, never route names. OPA + the domain engine both check; platform-only actions cannot reach tenant principals. |
| **API6 Unrestricted access to sensitive business flows** | Idempotency keys on writes; optimistic locking; lockout on repeated failures; every flow audited. |
| **API7 SSRF** | The service makes no user-controlled outbound requests; OPA/OpenFGA/Kafka endpoints come from configuration only. |
| **API8 Security misconfiguration** | Deny-by-default RLS, strict CORS, security headers, the app role cannot bypass RLS, refuses to start without a ≥32-char signing key. |
| **API9 Improper inventory management** | One versioned OpenAPI document generating the TypeScript client; `/healthz`, `/readyz`, `/metrics` explicit. |
| **API10 Unsafe consumption of third-party APIs** | OPA/OpenFGA responses are validated and fail closed; timeouts on every call. |

## Threats and answers

**Stolen refresh token** — usable once; the next use by either party revokes the
family and audits `refresh_token_reuse_detected`.

**XSS in the portal** — the refresh token is HttpOnly and unreadable; the access
token is in memory and expires in 15 minutes; nothing is in web storage.

**CSRF against `/auth/refresh`** — `SameSite=Strict` means cross-site requests
carry no cookie; the endpoint also needs no ambient authority beyond it.

**Malicious tenant admin escalating** — the delegation rule caps grants at what
they hold; platform-only permissions and `scope=all` are refused; system roles
are immutable; every attempt is audited even though the transaction rolls back.

**Compromised app database credentials** — the app role cannot bypass RLS, so
queries still need a tenant context; it also cannot UPDATE or DELETE audit rows.

**Insider erasing their tracks** — `audit_logs` blocks UPDATE/DELETE with a
trigger, and the app role holds INSERT only.

## Production checklist

- [ ] Secrets from a secret manager; `.env` never committed.
- [ ] `JWT_SIGNING_KEY` ≥ 32 chars of entropy; rotate with an overlap window.
- [ ] TLS everywhere; verify the refresh cookie carries `Secure` (set behind `X-Forwarded-Proto: https`).
- [ ] App connects as `wallet_app`, **not** the table owner — otherwise RLS silently does nothing.
- [ ] `OPENFGA_STORE_ID` and `OPENFGA_MODEL_ID` pinned.
- [ ] `RATE_LIMIT_RPS` tuned for real client IPs; `LOGIN_RATE_LIMIT_PER_MIN` kept strict.
- [ ] Kafka with TLS/SASL; auto topic creation off.
- [ ] Bootstrap admin password rotated; per-person platform accounts created.
- [ ] Audit retention, partitioning and archival planned.
- [ ] Alerts on `authz_decisions_total{decision="deny"}` spikes, lock rates, outbox lag, DLQ depth.
- [ ] `govulncheck`, `gosec`, `npm audit`, Trivy in CI (already wired) and reviewed.

## Reporting

Report suspected vulnerabilities privately to the platform security team. Do not
open a public issue.
