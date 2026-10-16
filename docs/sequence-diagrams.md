# Sequence diagrams

Every diagram reflects the code as it runs today.

## Login

Note where the transaction commits: a rejected sign-in still persists its
failed-attempt counter, its lock and its audit record. Returning the error from
inside the transaction would roll all three back — see ADR-005.

```mermaid
sequenceDiagram
    autonumber
    actor U as User
    participant P as Admin portal
    participant API as API (httpapi)
    participant RL as Redis (login limiter)
    participant AS as AuthService
    participant DB as PostgreSQL

    U->>P: email + password
    P->>API: POST /auth/login (X-Auth-Mode: cookie)
    API->>RL: Allow("login:<ip>", 10/min)
    alt limiter unavailable
        RL--xAPI: error
        API-->>P: 503 (fails closed — never open)
    else over the limit
        RL-->>API: false
        API-->>P: 429 RATE_LIMITED
    else within the limit
        RL-->>API: true
        API->>AS: Login(command)
        AS->>DB: BEGIN (app.is_platform = true)
        AS->>DB: find user + credential
        alt account locked
            AS->>DB: audit(denied, account_locked)
            AS->>DB: COMMIT
            AS-->>API: 403 ACCOUNT_LOCKED
        else wrong password
            AS->>DB: failed_login_attempts += 1
            opt 5th failure
                AS->>DB: status = locked, locked_until = now + 15m
            end
            AS->>DB: audit(failure, invalid_password)
            AS->>DB: COMMIT
            AS-->>API: 401 INVALID_CREDENTIALS
        else correct password
            AS->>DB: reset counters, create session (refresh hash)
            AS->>DB: audit(success)
            AS->>DB: COMMIT
            AS-->>API: access token + refresh token
        end
        API-->>P: 200 { access_token } + Set-Cookie: wg_refresh_token (HttpOnly, Strict)
    end
    P->>P: keep the access token in memory only
    opt must_change_password
        P->>U: force the password-change page
    end
```

## Authorization decision

```mermaid
sequenceDiagram
    autonumber
    participant H as Handler
    participant AZ as Authorizer
    participant R as Redis
    participant DB as PostgreSQL
    participant FGA as OpenFGA
    participant OPA as OPA
    participant D as Domain engine

    H->>AZ: Authorize(principal, {resource, action, target})
    AZ->>R: GET tenant:{t}:v{n}:principal:{p}:permissions
    alt cache hit
        R-->>AZ: permissions
    else miss or Redis down
        AZ->>DB: effective permissions (roles ∪ direct grants)
        DB-->>AZ: permissions
        AZ->>R: SET (TTL 2m, best effort)
    end

    AZ->>FGA: Check(user:<id>, member, tenant:<id>)
    alt OpenFGA unreachable
        FGA--xAZ: error
        AZ->>DB: audit(denied, relationship_check_unavailable)
        AZ-->>H: 503 (fail closed)
    else not related
        FGA-->>AZ: false
        AZ->>DB: audit(denied, cross_tenant_denied)
        AZ-->>H: 403
    else related
        FGA-->>AZ: true
        AZ->>OPA: decision(principal, request, target, permissions, relationships)
        alt OPA unreachable
            OPA--xAZ: error
            AZ-->>H: 503 (fail closed)
        else policy denies
            OPA-->>AZ: {allow: false, reason}
            AZ->>DB: audit(denied, reason)
            AZ-->>H: 403
        else policy allows
            OPA-->>AZ: {allow: true, allowed_fields, filters}
            AZ->>D: re-check the action permission (defence in depth)
            alt the engine disagrees with OPA
                D-->>AZ: false
                AZ-->>H: 403 permission_missing
            else agreed
                D-->>AZ: true + allowed fields
                AZ-->>H: Decision{allow, allowed_fields, permSet}
            end
        end
    end
```

## User creation

```mermaid
sequenceDiagram
    autonumber
    participant API as API
    participant IDEM as Redis (idempotency)
    participant US as UserService
    participant AZ as Authorizer
    participant DB as PostgreSQL
    participant FGA as OpenFGA

    API->>IDEM: claim Idempotency-Key
    alt key replayed with the same body
        IDEM-->>API: stored response
        API-->>API: replay it (no second user)
    else key reused with a different body
        IDEM-->>API: conflict
        API-->>API: 409 IDEMPOTENCY_CONFLICT
    else fresh key
        API->>US: Create(command, provided_fields)
        US->>US: reject unknown body fields (mass assignment)
        US->>AZ: MustAllow(users.create, target tenant)
        AZ-->>US: Decision
        US->>US: every protected field needs users.update.<field>
        US->>DB: BEGIN (RLS context)
        US->>DB: tenant is operational? email unique?
        US->>DB: INSERT user (must_change_password = true)
        US->>DB: INSERT credential (Argon2id hash)
        loop each requested role
            US->>DB: read the role's permissions
            US->>US: delegation rule — actor must hold them all
            US->>DB: INSERT user_role
        end
        US->>DB: INSERT audit_log
        US->>DB: INSERT outbox_events (user.created.v1)
        US->>DB: COMMIT
        US->>FGA: Write(user:<id> member tenant:<id>)
        US-->>API: 201 (fields filtered by read grants)
    end
```

## Permission assignment (delegation)

```mermaid
sequenceDiagram
    autonumber
    actor A as Actor
    participant API as API
    participant RS as RoleService
    participant AZ as Authorizer
    participant D as Domain engine
    participant DB as PostgreSQL
    participant R as Redis

    A->>API: POST /roles/{id}/permissions {permission_id, scope, effect}
    API->>RS: AddPermission
    RS->>AZ: MustAllow(permissions.assign, role's tenant)
    AZ-->>RS: Decision{permSet}
    RS->>RS: system role? → 403 (immutable)
    RS->>DB: BEGIN (RLS context)
    RS->>DB: load the permission definition
    RS->>RS: platform-only → 403 on a tenant role
    RS->>RS: scope=all → 403 on a tenant role
    RS->>D: CanDelegate(grant)
    alt not held / wider scope / looser conditions / denied
        D-->>RS: DelegationError
        RS->>DB: ROLLBACK
        RS->>DB: audit(denied, reason) in its OWN transaction
        RS-->>A: 403 PERMISSION_DELEGATION_DENIED
    else subset of what the actor holds
        D-->>RS: ok
        RS->>DB: INSERT role_permission
        RS->>DB: INSERT audit_log(success)
        RS->>DB: INSERT outbox_events (role.permission_assigned.v1)
        RS->>DB: COMMIT
        RS->>R: INCR tenant:{t}:permver  (invalidates every cached principal)
        RS-->>A: 201
    end
```

## API client token

```mermaid
sequenceDiagram
    autonumber
    participant C as API client
    participant API as API
    participant RL as Redis (limiter)
    participant OS as OAuthService
    participant DB as PostgreSQL

    C->>API: POST /oauth/token {grant_type, client_id, client_secret}
    API->>RL: Allow("oauth:<ip>", 10/min)
    API->>OS: Token(command)
    OS->>DB: BEGIN (platform context)
    OS->>DB: find client by client_id
    alt unknown / revoked / suspended / expired / IP not allow-listed
        OS->>DB: audit(denied)
        OS->>DB: COMMIT
        OS-->>C: 401 (uniform message — no client enumeration)
    else valid
        OS->>OS: Argon2id verify (constant time)
        alt wrong secret
            OS->>DB: audit(failure, invalid_client_secret)
            OS->>DB: COMMIT
            OS-->>C: 401
        else correct
            OS->>DB: tenant operational? touch last_used_at
            OS->>DB: audit(success)
            OS->>DB: COMMIT
            OS-->>C: 200 {access_token, token_type, expires_in, scope}
        end
    end
    Note over C,API: The token carries principal_type=api_client and no session_id.
```

## Outbox publish

```mermaid
sequenceDiagram
    autonumber
    participant TX as Business transaction
    participant DB as PostgreSQL
    participant W as Outbox worker
    participant K as Kafka

    TX->>DB: state change + INSERT outbox_events (same transaction)
    Note over TX,DB: Kafka is never touched here — a rollback loses the event too.

    loop every poll interval
        W->>DB: BEGIN
        W->>DB: SELECT ... WHERE status IN (pending, failed) AND next_attempt_at <= now()<br/>FOR UPDATE SKIP LOCKED
        loop each event
            W->>K: publish to wallet-gateway.<aggregate> (key = aggregate_id)
            alt published
                K-->>W: ack
                W->>DB: status = published
            else failed and attempts < max
                W->>DB: status = failed, attempts += 1,<br/>next_attempt_at = now + 2^attempt
            else attempts exhausted
                W->>K: publish to the dead-letter topic
                W->>DB: status = dead
            end
        end
        W->>DB: COMMIT
    end
```

## Cache invalidation

```mermaid
sequenceDiagram
    autonumber
    participant S as Service
    participant R as Redis
    participant AZ as Authorizer

    Note over R: Key: wallet-gateway:v1:tenant:{t}:v{n}:principal:{p}:permissions

    alt a principal's own grants change
        S->>R: DEL …principal:{p}:permissions
    else a role changes (affects many principals)
        S->>R: INCR tenant:{t}:permver   (n → n+1)
        Note over R: Every key under v{n} is now unreachable and expires by TTL —<br/>no scan, no key enumeration.
    end

    AZ->>R: GET tenant:{t}:permver → n+1
    AZ->>R: GET …v{n+1}:principal:{p}:permissions → miss
    AZ->>AZ: reload from PostgreSQL, cache under v{n+1}

    Note over AZ,R: If Redis is down, every read is a miss and PostgreSQL answers.<br/>Authorization degrades in latency, never in correctness.
```
