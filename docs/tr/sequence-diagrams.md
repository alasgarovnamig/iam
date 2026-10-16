# Sıralama diyagramları

Her diyagram, kodun bugün nasıl çalıştığını yansıtır.

## Giriş (login)

Transaction'ın nerede commit ettiğine dikkat et: reddedilen bir giriş bile
başarısız-deneme sayacını, kilidini ve audit kaydını yine de kalıcı kılar. Hatayı
transaction'ın içinden döndürmek üçünü de geri alırdı — bkz. ADR-005.

```mermaid
sequenceDiagram
    autonumber
    actor U as Kullanıcı
    participant P as Admin portal
    participant API as API (httpapi)
    participant RL as Redis (login limiter)
    participant AS as AuthService
    participant DB as PostgreSQL

    U->>P: e-posta + şifre
    P->>API: POST /auth/login (X-Auth-Mode: cookie)
    API->>RL: Allow("login:<ip>", 10/dk)
    alt limiter erişilemez
        RL--xAPI: hata
        API-->>P: 503 (fail-closed — asla açık değil)
    else limitin üstünde
        RL-->>API: false
        API-->>P: 429 RATE_LIMITED
    else limit içinde
        RL-->>API: true
        API->>AS: Login(command)
        AS->>DB: BEGIN (app.is_platform = true)
        AS->>DB: kullanıcı + credential bul
        alt hesap kilitli
            AS->>DB: audit(denied, account_locked)
            AS->>DB: COMMIT
            AS-->>API: 403 ACCOUNT_LOCKED
        else yanlış şifre
            AS->>DB: failed_login_attempts += 1
            opt 5. başarısızlık
                AS->>DB: status = locked, locked_until = now + 15dk
            end
            AS->>DB: audit(failure, invalid_password)
            AS->>DB: COMMIT
            AS-->>API: 401 INVALID_CREDENTIALS
        else doğru şifre
            AS->>DB: sayaçları sıfırla, session yarat (refresh hash)
            AS->>DB: audit(success)
            AS->>DB: COMMIT
            AS-->>API: access token + refresh token
        end
        API-->>P: 200 { access_token } + Set-Cookie: wg_refresh_token (HttpOnly, Strict)
    end
    P->>P: access token'ı yalnızca bellekte tut
    opt must_change_password
        P->>U: şifre-değiştirme sayfasını zorla
    end
```

## Yetkilendirme kararı

```mermaid
sequenceDiagram
    autonumber
    participant H as Handler
    participant AZ as Authorizer
    participant R as Redis
    participant DB as PostgreSQL
    participant FGA as OpenFGA
    participant OPA as OPA
    participant D as Domain motoru

    H->>AZ: Authorize(principal, {resource, action, target})
    AZ->>R: GET tenant:{t}:v{n}:principal:{p}:permissions
    alt cache isabet
        R-->>AZ: izinler
    else ıskalama ya da Redis kapalı
        AZ->>DB: efektif izinler (roller ∪ doğrudan grant'lar)
        DB-->>AZ: izinler
        AZ->>R: SET (TTL 2dk, best effort)
    end

    AZ->>FGA: Check(user:<id>, member, tenant:<id>)
    alt OpenFGA erişilemez
        FGA--xAZ: hata
        AZ->>DB: audit(denied, relationship_check_unavailable)
        AZ-->>H: 503 (fail closed)
    else ilişkili değil
        FGA-->>AZ: false
        AZ->>DB: audit(denied, cross_tenant_denied)
        AZ-->>H: 403
    else ilişkili
        FGA-->>AZ: true
        AZ->>OPA: decision(principal, request, target, permissions, relationships)
        alt OPA erişilemez
            OPA--xAZ: hata
            AZ-->>H: 503 (fail closed)
        else politika reddediyor
            OPA-->>AZ: {allow: false, reason}
            AZ->>DB: audit(denied, reason)
            AZ-->>H: 403
        else politika izin veriyor
            OPA-->>AZ: {allow: true, allowed_fields, filters}
            AZ->>D: action iznini yeniden kontrol et (defence in depth)
            alt motor OPA ile anlaşmıyor
                D-->>AZ: false
                AZ-->>H: 403 permission_missing
            else anlaşıldı
                D-->>AZ: true + izinli alanlar
                AZ-->>H: Decision{allow, allowed_fields, permSet}
            end
        end
    end
```

## Kullanıcı oluşturma

```mermaid
sequenceDiagram
    autonumber
    participant API as API
    participant IDEM as Redis (idempotency)
    participant US as UserService
    participant AZ as Authorizer
    participant DB as PostgreSQL
    participant FGA as OpenFGA

    API->>IDEM: Idempotency-Key talep et
    alt anahtar aynı gövdeyle tekrar oynatıldı
        IDEM-->>API: saklanan cevap
        API-->>API: tekrar oynat (ikinci kullanıcı yok)
    else anahtar farklı gövdeyle yeniden kullanıldı
        IDEM-->>API: çakışma
        API-->>API: 409 IDEMPOTENCY_CONFLICT
    else taze anahtar
        API->>US: Create(command, provided_fields)
        US->>US: bilinmeyen gövde alanlarını reddet (mass assignment)
        US->>AZ: MustAllow(users.create, hedef tenant)
        AZ-->>US: Decision
        US->>US: her korumalı alan users.update.<field> gerektirir
        US->>DB: BEGIN (RLS bağlamı)
        US->>DB: tenant operasyonel mi? e-posta unique mi?
        US->>DB: INSERT user (must_change_password = true)
        US->>DB: INSERT credential (Argon2id hash)
        loop her istenen rol
            US->>DB: rolün izinlerini oku
            US->>US: delegasyon kuralı — actor hepsini tutmalı
            US->>DB: INSERT user_role
        end
        US->>DB: INSERT audit_log
        US->>DB: INSERT outbox_events (user.created.v1)
        US->>DB: COMMIT
        US->>FGA: Write(user:<id> member tenant:<id>)
        US-->>API: 201 (alanlar read grant'larına göre filtreli)
    end
```

## İzin ataması (delegasyon)

```mermaid
sequenceDiagram
    autonumber
    actor A as Actor
    participant API as API
    participant RS as RoleService
    participant AZ as Authorizer
    participant D as Domain motoru
    participant DB as PostgreSQL
    participant R as Redis

    A->>API: POST /roles/{id}/permissions {permission_id, scope, effect}
    API->>RS: AddPermission
    RS->>AZ: MustAllow(permissions.assign, rolün tenant'ı)
    AZ-->>RS: Decision{permSet}
    RS->>RS: system rol mü? → 403 (değiştirilemez)
    RS->>DB: BEGIN (RLS bağlamı)
    RS->>DB: izin tanımını yükle
    RS->>RS: platform-only → tenant rolünde 403
    RS->>RS: scope=all → tenant rolünde 403
    RS->>D: CanDelegate(grant)
    alt tutulmuyor / daha geniş scope / daha gevşek koşul / reddedildi
        D-->>RS: DelegationError
        RS->>DB: ROLLBACK
        RS->>DB: audit(denied, reason) KENDİ transaction'ında
        RS-->>A: 403 PERMISSION_DELEGATION_DENIED
    else actor'ın tuttuğunun alt kümesi
        D-->>RS: ok
        RS->>DB: INSERT role_permission
        RS->>DB: INSERT audit_log(success)
        RS->>DB: INSERT outbox_events (role.permission_assigned.v1)
        RS->>DB: COMMIT
        RS->>R: INCR tenant:{t}:permver  (her cache'lenmiş principal'i invalidate eder)
        RS-->>A: 201
    end
```

## API client token'ı

```mermaid
sequenceDiagram
    autonumber
    participant C as API client
    participant API as API
    participant RL as Redis (limiter)
    participant OS as OAuthService
    participant DB as PostgreSQL

    C->>API: POST /oauth/token {grant_type, client_id, client_secret}
    API->>RL: Allow("oauth:<ip>", 10/dk)
    API->>OS: Token(command)
    OS->>DB: BEGIN (platform bağlamı)
    OS->>DB: client_id ile client bul
    alt bilinmeyen / iptal / askıya alınmış / süresi dolmuş / IP allow-list'te değil
        OS->>DB: audit(denied)
        OS->>DB: COMMIT
        OS-->>C: 401 (tek tip mesaj — client sayımı yok)
    else geçerli
        OS->>OS: Argon2id doğrula (sabit zaman)
        alt yanlış secret
            OS->>DB: audit(failure, invalid_client_secret)
            OS->>DB: COMMIT
            OS-->>C: 401
        else doğru
            OS->>DB: tenant operasyonel mi? last_used_at'i güncelle
            OS->>DB: audit(success)
            OS->>DB: COMMIT
            OS-->>C: 200 {access_token, token_type, expires_in, scope}
        end
    end
    Note over C,API: Token, principal_type=api_client taşır ve session_id taşımaz.
```

## Outbox yayını

```mermaid
sequenceDiagram
    autonumber
    participant TX as İş transaction'ı
    participant DB as PostgreSQL
    participant W as Outbox worker
    participant K as Kafka

    TX->>DB: state değişimi + INSERT outbox_events (aynı transaction)
    Note over TX,DB: Kafka'ya burada asla dokunulmaz — bir rollback event'i de kaybeder.

    loop her poll aralığı
        W->>DB: BEGIN
        W->>DB: SELECT ... WHERE status IN (pending, failed) AND next_attempt_at <= now()<br/>FOR UPDATE SKIP LOCKED
        loop her event
            W->>K: wallet-gateway.<aggregate>'e yayınla (key = aggregate_id)
            alt yayınlandı
                K-->>W: ack
                W->>DB: status = published
            else başarısız ve attempts < max
                W->>DB: status = failed, attempts += 1,<br/>next_attempt_at = now + 2^attempt
            else denemeler tükendi
                W->>K: dead-letter topic'ine yayınla
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
    participant S as Servis
    participant R as Redis
    participant AZ as Authorizer

    Note over R: Anahtar: wallet-gateway:v1:tenant:{t}:v{n}:principal:{p}:permissions

    alt bir principal'in kendi grant'ları değişir
        S->>R: DEL …principal:{p}:permissions
    else bir rol değişir (birçok principal'i etkiler)
        S->>R: INCR tenant:{t}:permver   (n → n+1)
        Note over R: v{n} altındaki her anahtar artık erişilemez ve TTL ile sona erer —<br/>tarama yok, anahtar sayımı yok.
    end

    AZ->>R: GET tenant:{t}:permver → n+1
    AZ->>R: GET …v{n+1}:principal:{p}:permissions → ıskalama
    AZ->>AZ: PostgreSQL'den yeniden yükle, v{n+1} altında cache'le

    Note over AZ,R: Redis kapalıysa her okuma bir ıskalamadır ve PostgreSQL cevap verir.<br/>Yetkilendirme gecikmede bozulur, asla doğrulukta değil.
```
