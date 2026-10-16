# Çok-kiracılık (multi-tenancy)

## Taraflar

| Taraf | Anlamı |
|---|---|
| **Platform** | Biz. Platform kullanıcılarının `tenant_id = NULL`'dır ve kiracılar arası çalışabilir. |
| **Tenant / partner** | Bir müşteri şirket. Kullanıcıları, API client'ları, rolleri, izinleri, audit log'ları, organizasyon birimleri ve verisi onun içinde yaşar. |

Her kiracı-scope'lu tablo `tenant_id` taşır. Bir kiracı başka bir kiracının
verisini göremez — ne listeleyerek, ne id tahmin ederek, ne unutulmuş bir
`WHERE` ile.

## Üç bağımsız sınır

| Katman | Uygulama |
|---|---|
| Uygulama | `dbCtxFor(principal)`, açık kiracı kontrolleri, `Başka kiracıda kullanıcı yaratamazsın` |
| OPA | Platform-olmayan principal'ler için `target.tenant_id != principal.tenant_id` olduğunda `cross_tenant_violation` |
| PostgreSQL | Her kiracı-scope'lu tabloda Row-Level Security politikaları |

Defence in depth amaçtır: herhangi bir katman sızıntıyı durdururdu; üçü, tek bir
hatanın sızıntıya yol açamayacağı anlamına gelir.

## Row-Level Security

Uygulama, tabloların **sahibi olmayan** `wallet_app` rolüyle bağlanır — RLS tablo
sahiplerine uygulanmaz, izolasyonu bir veritabanı garantisi yapan bu ayrımdır.
Migration'lar sahip rolünü kullanmaya devam eder.

Her iş birimi, bağlamı transaction içinde ayarlar:

```sql
-- tenant principal'leri
SELECT set_config('app.tenant_id', '<uuid>', true);
-- platform principal'leri ve worker'lar
SELECT set_config('app.is_platform', 'true', true);
```

Politikalar bu GUC'leri okur:

```sql
CREATE POLICY users_tenant ON users
    USING (tenant_id = app_tenant_id())
    WITH CHECK (tenant_id = app_tenant_id());

CREATE POLICY users_platform_all ON users
    USING (app_is_platform()) WITH CHECK (app_is_platform());
```

Hiçbir GUC set edilmezse **hiçbir şey görünmez**. Depolama katmanında varsayılan
olarak reddet.

`SET LOCAL` yalnızca bir transaction içinde var olduğundan, repo'lar
`TxManager.WithTx` dışında çalışmayı reddeder — transaction'sız bir sorgu kiracı
bağlamı taşımaz, ki bu tam olarak RLS'in var olma nedeni olan bug'dır.

Doğrulanmış davranış:

| Bağlam | `SELECT count(*) FROM users` |
|---|---|
| yok | 0 |
| `app.tenant_id = A` | yalnızca A'nın kullanıcıları |
| `app.tenant_id = A`, B'nin kullanıcısını **tam id'siyle** seçerken | 0 |
| `app.is_platform = true` | hepsi |

## Kiracılar arası system roller

System roller `tenant_id IS NULL`'da yaşar ve her kiracı tarafından okunabilir
(`roles_system_read`), ama hiç kimse tarafından yazılamaz:
`chk_system_roles_global` artı her yazma yolundaki `is_system = false`
korumaları. Kiracılar bunun yerine kendi custom rollerini yaratır.

## Kiracılar arası giriş

Giriş, henüz kiracı kimliği olmayan tek yerdir, o yüzden platform bağlamında
çalışır. Çözümleyici bir platform hesabını tercih eder; bir e-posta birden çok
kiracıda varsa çağıran `tenant_slug` sağlamalıdır.

## Cache ve event'ler

Cache anahtarları kiracı-scope'lu ve versiyonludur:
`wallet-gateway:v1:tenant:{tenantId}:v{n}:principal:{p}:permissions`. Kafka
event'leri zarfta `tenant_id` taşır ve outbox satırı da tutar, böylece downstream
consumer'lar kiracıya göre partition/filtre yapabilir.

## Organizasyon birimleri

`organization_units`, her kiracıya `team`, `organization` ve `descendants`
scope'ları için bir ağaç verir. Tablo, FGA tipi ve scope semantiği vardır; mevcut
API henüz birimler için CRUD sunmaz — bu bir sonraki artımdır ve şema değişikliği
gerektirmez.
