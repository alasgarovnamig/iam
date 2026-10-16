# Yetkilendirme

Üç katman; üçü de aynı fikirde olmalı, üçü de fail-closed.

| Katman | Soru | Yer |
|---|---|---|
| OpenFGA | Principal hedefle *ilişkili* mi? | `internal/openfga` + `model.fga` |
| OPA | Politika bu *isteğe* izin veriyor mu, hangi alanları? | `deployments/opa/policies/authz.rego` |
| Domain motoru | *İzinler* bu action'ı ve bu alanları gerçekten kapsıyor mu? | `domain/permission.go` |

Varsayılan olarak reddet: bir kural izin vermedikçe hiçbir şey izinli değildir.

## İzin modeli

```
service . resource . action [ . field ]   +  scope  +  effect  +  conditions
```

* **service** — bugün `user-management`; ileriki servisler kendilerininkini ekler.
* **resource** — `users`, `roles`, `tenants`, `api-clients`, `audit-logs`, `sessions`, `permissions`.
* **action** — `create`, `read`, `update`, `delete`, `search`, `list`, `assign`, `revoke`, `export`, `rotate`.
* **field** — opsiyonel; grant'ı field-level yapar.
* **scope** — `own` · `assigned` · `team` · `organization` · `descendants` · `tenant` · `all` (dar → geniş).
* **effect** — `allow` veya `deny`. **Deny her zaman kazanır**, hedefe ulaşan herhangi bir scope'ta.
* **conditions** — grant'ın karşılaması gereken anahtar/değer kısıtları.

Örnekler:

```
user-management.users.create
user-management.users.read.email
user-management.users.update.first_name
user-management.users.search.phone
user-management.roles.assign
user-management.api-clients.create
```

## Field-level izinler

Bir action grant'ı temel alanları kapsar (`id`, `tenant_id`, `user_type`,
timestamp'ler, `version`). Her korumalı alan kendi grant'ını gerektirir:

* **Read** — `users.read.<field>` olmayan alanlar cevaptan **çıkarılır**, null
  yapılmaz. Secret materyal (`password_hash`, `refresh_token_hash`) hiçbir izne
  sahip değildir ve veritabanından hiç çıkmaz.
* **Update/Create** — `users.update.<field>` olmadan bir alan taşıyan gövde
  `403 FIELD_NOT_ALLOWED` ile reddedilir. Bilinmeyen alanlar `400`.
* **Search** — bir alanla filtrelemek `users.search` **ve** `users.search.<field>`
  gerektirir. Bir alanı okumak, onunla aramayı ima etmez: bir destek elemanı bir
  müşterinin e-postasını görebilir ama kullanıcı tabanını e-postayla sayamaz.

## Scope'lar

| Scope | Ulaşır |
|---|---|
| `own` | Yalnızca principal'ın kendisi |
| `assigned` | Principal'a açıkça atanmış kaynaklar |
| `team` | Principal'ın doğrudan organizasyon birimi |
| `organization` | Principal'ın organizasyon alt-ağacı |
| `descendants` | Principal'ın biriminin altındaki alt-ağaç |
| `tenant` | Principal'ın kiracısındaki her şey |
| `all` | Platform genelinde (yalnızca platform principal'leri) |

Koleksiyon istekleri (`create`, `list`, `search`) somut hedef satırı taşımaz;
bu yüzden tenant-geneli bir grant, principal'ın kendi kiracısı içinde bunlara
ulaşır.

## Karar hattı

```
authenticate (JWT: yalnızca kimlik, izin yok)
  → effective izinleri yükle   (Redis → PostgreSQL; miss ≠ allow)
  → OpenFGA Check              (kiracının/platformun üyesi mi?)
  → OPA kararı                 (kiracı sınırı, izin eşleştirme, alanlar, filtreler)
  → domain yeniden-kontrol     (defence in depth; anlaşmazlık reddeder)
  → handler                    (cevap izinli alanlara filtrelenir)
```

Kiracı sınırı üç bağımsız yerde kontrol edilir: uygulama (`Başka kiracıda
kullanıcı yaratamazsın`), OPA (`cross_tenant_violation`) ve PostgreSQL RLS.
Herhangi biri tek başına sızıntıyı durdururdu; üçü birlikte tek bir hatanın
sızıntıya yol açamayacağı anlamına gelir.

## OPA

Girdi:

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

Çıktı:

```json
{
  "allow": true,
  "reason": "permission_granted",
  "allowed_fields": ["email", "first_name"],
  "denied_fields": ["internal_notes"],
  "filters": { "tenant_id": "…" }
}
```

Nedenler: `permission_granted`, `permission_missing`, `scope_insufficient`,
`explicit_deny`, `cross_tenant_denied`, `permission_denied`.

Politika testlerini `make opa-test` ile çalıştır (15 senaryo: deny-by-default,
kiracı izolasyonu, deny önceliği, scope değerlendirmesi, field çıktısı, satır
filtreleri).

## OpenFGA

Yalnızca ilişkileri tutar. Tipler: `platform`, `tenant`, `organization_unit`,
`role`, `user`, `api_client`, `resource`. Servisin isimle bağlı olduğu
ilişkiler: `platform#member`, `platform#super_admin`, `tenant#member`,
`role#assignee`.

Model tek bir DSL dosyasıdır, boot'ta gömülüp uygulanır (ADR-006). `resource`
zaten `tenant`/`owner`/`manager`/`viewer` taşır, böylece ileriki servisler model
yeniden yazmadan ona bağlanır.

## Hata davranışı

| Bağımlılık | Kapalı | Davranış |
|---|---|---|
| OpenFGA | ✕ | `503`, istek reddedilir, red audit'lenir |
| OPA | ✕ | `503`, istek reddedilir, red audit'lenir |
| Redis (izin cache) | ✕ | Cache miss → PostgreSQL; **asla** örtük allow |
| Redis (login limiter) | ✕ | Giriş reddedilir (`503`) — Redis'te fail-closed olduğumuz tek yer |
| Redis (genel limiter) | ✕ | İstekler devam eder; auth-dışı trafikte erişilebilirlik kazanır |
| PostgreSQL | ✕ | `500`/`503` — doğruluk kaynağı olmadan hiçbir şey yetkilendirilmez |

## Cache

```
wallet-gateway:v1:tenant:{tenantId}:v{version}:principal:{principalId}:permissions   TTL 2dk
```

Versiyonlu ve kiracı-farkında. Bir rol değişimi `tenant:{t}:permver`'i bump'lar,
böylece o kiracıdaki tüm cache'li principal'ler bir kerede geçersiz kılınır —
anahtar taraması yok. Doğrudan grant'lar yalnızca o principal'ın anahtarını siler.
