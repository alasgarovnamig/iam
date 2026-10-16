# Onboarding — okuduğun ilk belge bu olsun

Bu doküman, ekibe yeni katılan bir geliştiricinin projeyi **doğru sırayla** anlaması
ve ilk katkısını güvenle yapabilmesi için yazıldı. Sırayla oku, atlama.

---

## 0. Tek cümlelik zihinsel model

> Bu proje bir **kimlik + yetki + çok-kiracılık (multi-tenancy)** temelidir. Kalbi
> şu soruya güvenli cevap vermek: **"kim, hangi kiracıda, neyi, hangi alanına kadar
> yapabilir?"** Cüzdan/ödeme modülleri henüz yok; biz onların üstüne oturacağı zemini
> kurduk.

Bu cümleyi içselleştirmeden koda dalma. Buradaki her tasarım kararı bu soruya hizmet
ediyor.

---

## 1. Bağımlılık kuralı (her şeyin anahtarı)

Mimari **hexagonal (ports & adapters)**. Tek kuralı ezberle: **bağımlılık oku hep
içeri bakar.**

```
internal/httpapi  →  internal/app  →  internal/domain
  (HTTP, JSON)        (use-case)        (saf kural)
       │                   │                 ▲
       │                   └─ internal/ports ┤
       │                       (interface)   │
       └─ internal/postgres · cache · kafka ─┘
          internal/opa · openfga
          (bu interface'leri implement eder)
```

Tüm paketler `internal/` altında **tek seviye** durur. Bir paketin adı ne
olduğunu söyler: Postgres'e giden `postgres`, Redis'e giden `cache`.

| Paket | Ne YAPAR | Ne YAPMAZ |
|---|---|---|
| `internal/domain/` | Kural: "deny allow'u yener", "veremediğini veremezsin", durum makineleri | `import pgx`, `net/http`, JWT — **hiçbir dış kütüphane** |
| `internal/ports/` | Domain'in dış dünyadan istediği interface'ler | Implementasyon barındırmaz |
| `internal/app/` | Transaction aç, yetki sor, audit yaz, event üret, cache temizle | SQL string yazmaz, `http.Request` görmez |
| `internal/postgres/` · `httpapi/` · `cache/` · `kafka/` · `opa/` · `openfga/` | pgx sorguları, chi handler'ları, Redis anahtarları, OPA/OpenFGA HTTP | İş kuralı barındırmaz |

İki cümle sürekli tekrarlanacak:
- **"SQL'i handler'a yazma."** Handler sadece tercüman: JSON al → servisi çağır → yaz.
- **"Domain'e framework sokma."** Sokarsan test edilemez hale gelir. Kanıt: `internal/domain`
  testleri hiçbir fake/mock kullanmadan çalışır.

---

## 2. Okuma sırası (içeriden dışarıya)

Yanlış yol: `cmd/api/main.go`'dan başlamak. Doğru yol:

1. **`docs/architecture.md`** — ADR'ler (karar kayıtları). "Neden böyle?" cevapları burada. ~20 dk.
2. **`internal/domain/permission.go`** — **projedeki en önemli dosya.** Permission modeli +
   delegation kuralı (`CanDelegate`). Bunu anlamadan gerisi anlaşılmaz.
3. **`internal/domain/`** geri kalanı — `user.go`, `role.go`, `tenant.go` (aggregate'ler,
   durum makineleri).
4. **`internal/ports/repositories.go` + `ports/authz.go`** — "sistem dış dünyadan ne istiyor" listesi.
5. **`internal/app/authorizer.go`** — 3 katmanlı yetki orkestrasyonu.
6. **`internal/app/user_service.go`** — bir use-case'in baştan sona akışı.
7. **`internal/postgres/`, `cache/`, `opa/`, `openfga/`, `httpapi/`** — en sona. Burası "detay".
8. **`migrations/`** — `0004` (permission/rol) ve `0007` (RLS) kritik.

---

## 3. Bir isteğin hayatı — bunu bir kez debugger'la izle, her şey oturur

Örnek: `POST /api/v1/users`

1. **`handlers_resources.go` → `CreateUser`** — JSON'u parse eder, gönderilen alanların
   listesini (mass-assignment koruması) çıkarır, `UserService.Create`'i çağırır. İş kuralı yok.
2. **`user_service.go` → `Create`**:
   - Bilinmeyen alan → 400 (mass assignment).
   - `az.MustAllow(...)` → yetki (aşağıda 3 katman).
   - Field-level: `internal_notes` gönderdin ama `users.update.internal_notes` yoksa → 403.
   - `Tx.WithTx(...)` içinde: kullanıcı + credential yaz, rolleri **delegation'dan geçirerek**
     ata, `recordAudit`, `emitEvent` (outbox), **commit**.
   - Transaction dışında: OpenFGA ilişkisi yaz, cache temizle.
3. **`authorizer.go` → `Authorize`** (üçü de "evet" demeli):
   - **OpenFGA** → bu kişi bu kiracının üyesi mi?
   - **OPA** → politika izin veriyor mu, hangi alanları görebilir?
   - **Domain engine** → OPA ile aynı sonuç mu (defence in depth)?
4. **Response** — okuyamadığın alanlar cevaptan **silinir** (`dto.go → userDTO`).

> Pratik: `cmd/api`'yi debugger'la başlat, `UserService.Create`'e breakpoint koy, portaldan
> bir kullanıcı yarat, adım adım geç. 100 sayfa dokümandan iyidir.

---

## 4. Rol ve Permission — en çok karıştırılan yer

### Permission = en küçük yetki birimi

```
service . resource . action [ . field ]  +  scope  +  effect  +  conditions
```

| Parça | Ne | Örnek |
|---|---|---|
| service | hangi servis | `user-management` |
| resource | neyin üzerinde | `users`, `roles`, `tenants`, `api-clients` |
| action | ne yapıyor | `create`, `read`, `update`, `delete`, `search`, `list`, `assign`, `revoke`, `rotate` |
| field | hangi alan (opsiyonel) | `email`, `phone` → `users.read.email` |
| scope | **ne kadar geniş** | `own < team < organization < descendants < tenant < all` |
| effect | `allow` / `deny` | **deny her zaman kazanır** |

Örnek: `user-management.users.read.email` @ `scope=tenant` = "kendi kiracımdaki
kullanıcıların e-postasını okuyabilirim."

**Üç ayrı yetenek olduğunu unutma:**
- **read** — alanı görmek. İzin yoksa alan cevapta **hiç yoktur** (boş değil, yok).
- **update** — alanı yazmak. İzinsiz alan gönderirsen **tüm istek reddedilir** (`FIELD_NOT_ALLOWED`).
- **search** — o alanla filtrelemek. `read.email` olsa bile `search.email` yoksa e-postayla
  arama yapamazsın (bir müşterinin e-postasını görmek ≠ tüm listeyi e-postayla avlamak).

### Role = permission demeti

Rol tek başına iş yapmaz; içine permission konur. Kullanıcıya rol atarsın, o roldeki tüm
permission'ları alır.

- **System roller** (`is_system=true`, `tenant_id=NULL`) → platformun tanımladığı, **değiştirilemez**
  hazır roller. `migrations/0008_seed_catalog.up.sql`'de seed edilir. Sabit UUID'leri vardır.
- **Custom roller** → her kiracının kendi yarattığı roller.

### System roller ve sabit UUID'leri — hangisi kime hizmet ediyor

**Platform tarafı (bizim şirket):**

| UUID | Rol | Amaç |
|---|---|---|
| `a1000000-…-001` | Super Admin | Her şey. Bootstrap admin bu rolü taşır. |
| `a1000000-…-002` | Platform Admin | Platform yönetimi (birkaç hassas alan hariç) |
| `a1000000-…-003` | Operations Admin | Kiracı/kullanıcı operasyonları |
| `a1000000-…-004` | Support Agent | Çoğunlukla okuma; e-posta ile arama yok |
| `a1000000-…-005` | Compliance Officer | Denetim + uyumluluk |
| `a1000000-…-006` | Platform Auditor | Salt okunur + audit export |
| `a1000000-…-007` | Platform Read Only | Salt okunur |

**Kiracı tarafı (müşteri şirket):**

| UUID | Rol | Amaç |
|---|---|---|
| `a2000000-…-001` | Tenant Owner | Kiracıda tam yetki |
| `a2000000-…-002` | Tenant Admin | Kiracı yönetimi (owner'dan biraz dar) |
| `a2000000-…-003` | Tenant Developer | API client / entegrasyon |
| `a2000000-…-004` | Tenant Operations | Kullanıcı operasyonları |
| `a2000000-…-005` | Tenant Finance | Kullanıcı + audit okuma |
| `a2000000-…-006` | Tenant Support | Salt okuma (sınırlı alanlar) |
| `a2000000-…-007` | Tenant Auditor | Salt okunur |
| `a2000000-…-008` | Tenant Read Only | Salt okunur |
| `a2000000-…-009` | API Integration Manager | Sadece API client yönetimi |

### Delegation — projedeki EN kritik business kuralı

> **Bir kişi yalnızca kendinde olan yetkiyi, kendi scope'undan dar veya eşit olarak
> başkasına verebilir.**

- Sende `users.delete` yoksa → kimseye `users.delete` veremezsin.
- Sende `scope=tenant` varsa → `scope=all` **veremezsin** (daha geniş).
- Sende `users.read.email` varsa `users.read.phone` **veremezsin** (farklı alan).
- Action-level grant, field-level'i **ima etmez** (ve tersi).

Kod: `domain/permission.go → CanDelegate` (satır bazlı test: `permission_test.go`, 18 senaryo).
Detay: `docs/permission-delegation.md`. Bir rol atamak, o roldeki **tüm** permission'ları
delege etmek demektir — bu yüzden rol atama da bu kuraldan geçer (`CanDelegateAll`).

Reddedilen bir delegasyon → `403 PERMISSION_DELEGATION_DENIED` **ve** denemenin kendisi
audit'e yazılır (yetki büyütme girişimi asla kaybolmaz).

---

## 5. Yeni geliştirmeyi entegre etme — reçeteler

Önce **hangi senaryoda** olduğunu belirle, sonra reçeteyi izle. Sıra **hep içeriden dışarıya**.

### Reçete A — Var olan kaynağa yeni endpoint (ör. `POST /users/{id}/reset-mfa`)

1. **Domain** — yeni kural gerekiyorsa ekle (çoğu zaman gerekmez).
2. **Application** — `user_service.go`'ya metod. İç iskelet **her zaman** şu sırada:
   ```
   az.MustAllow(...)              // yetki
   Tx.WithTx(dbCtxFor(p), ...) {  // RLS context'li transaction
       ... değişiklik ...
       recordAudit(...)           // denetim kaydı
       emitEvent(...)             // outbox event (Kafka'ya buradan gitmez!)
   }
   az.Invalidate...(...)          // cache temizle
   ```
3. **Port** — yeni repo metodu lazımsa `ports/repositories.go`'ya interface satırı.
4. **Adapter** — `internal/postgres/user_repo.go`'da SQL'i yaz. **Parametreli sorgu**, sort/filter
   alanları **allow-list**.
5. **Handler** — `handlers_resources.go`'ya ince handler (parse → servis → yaz).
6. **Route** — `router.go`'ya bir satır.
7. **OpenAPI** — `openapi.yaml`'a endpoint → `make openapi` (frontend tipleri yenilenir).
8. **Test** — domain unit + `tests/integration`'a bir senaryo.

### Reçete B — Yeni permission

1. Yeni migration: `permission_definitions`'a satır (`INSERT ... ON CONFLICT DO NOTHING`).
2. Aynı migration'da hangi system roller tutmalı → `role_permissions`'a ekle.
3. Frontend: `web/src/auth/permissions.ts` → `P` sabitine kodu ekle (menü/buton gizleme).
4. Serviste `az.MustAllow`'a yeni resource/action'ı geç.

> Permission "endpoint adı" değildir. **Asla route adına göre yetki verme** — hep `resource.action`.

### Reçete C — Yeni kaynak (ör. `webhooks`)

B + A'nın birleşimi, ek olarak:
- Migration: tablo + **`tenant_id`** + **RLS policy** (`0007_rls.up.sql`'deki bir bloğu kopyala:
  `_platform_all` + `_tenant` policy'leri, `wallet_app`'e grant).
- Domain aggregate, port, postgres repo, application servis (audit+event), handler+route, OpenAPI.

### Reçete D — Yepyeni servis (ör. `wallet-service`)

Bu repo **tek servis, tek modül**. İkinci servis buraya klasör olarak eklenmez;
kendi repo'sunu alır ve aynı iç şekli tekrar eder:
- Aynı paket düzeni: `internal/domain`, `internal/ports`, `internal/app`, sonra adapter paketleri.
- İhtiyacı olan parçaları (auth, apperr, events, observability) oradan import eder ya da kopyalar —
  ikinci tüketici ortaya çıkmadan "ortak kütüphane" kurma.
- Kendi permission'larını `service: "wallet"` ile tanımla.
- OpenFGA'daki hazır `resource` tipine bağlan (zaten `tenant/owner/manager/viewer` ilişkileri var).
- Tenant, user, API client **zaten var** — kimliği tüketirsin, yeniden yazmazsın.

---

## 6. Altın kurallar (ekipte yola çıkmadan ezberle)

Bunlar teoriden değil, bu projede gerçekten düşülmüş tuzaklardan:

1. **İş reddi (business rejection) side-effect'lerini commit etmeli.** Yanlış şifre sayacı,
   hesap kilidi, refresh reuse iptali, escalation audit'i → transaction closure'ından `error`
   dönersen bunlar **geri alınır** ve güvenlik sessizce çalışmaz. Doğru kalıp: reddi `outcome`
   değişkeninde taşı, **commit'ten sonra** döndür. (`auth_service.go`'daki `outcome` deseni; ADR-005.)
2. **Her repo çağrısı `TxManager.WithTx` içinde olmalı.** Transaction dışı sorgu RLS context'i
   taşımaz → repo zaten reddeder (`tx.go`).
3. **Authorization fail-closed.** OPA/OpenFGA erişilemezse → 503, asla "geç". Redis düşerse →
   cache miss → DB (fail-soft); ama login limiter fail-closed.
4. **JWT'ye permission koyma.** Kimlik taşır, yetki değil — yetki her istekte canlı çözülür,
   böylece iptal anında etki eder.
5. **Rol/izin değişince cache temizle.** Rol değişikliği → tenant permver bump
   (`InvalidateTenantPermissions`); direkt grant → principal key sil (`InvalidatePermissions`).
6. **Cevaptan izinsiz alanı sil; request'te izinsiz alanı reddet.** İki ayrı yön, aynı prensip.
7. **Sistem rolleri değiştirilemez.** Migration dışında dokunma; kod da (`role.EnsureMutable`) engeller.
8. **Placeholder / `TODO: implement` bırakma.** İş bitmemişse merge etme.

---

## 7. İlk hafta planı

| Gün | Yapılacak |
|---|---|
| 1 | `docker compose up --build`. Portala gir (README'deki bootstrap admin), tenant + kullanıcı + custom rol yarat. OPA Viewer'da (`localhost:3020`) bir kararı canlı dene. |
| 2 | `docs/`: architecture → authorization → permission-delegation sırasıyla oku. |
| 3 | Bir isteği debugger'la baştan sona izle (Bölüm 3). |
| 4–5 | Küçük görev: Reçete A ile var olan bir kaynağa ufak endpoint ekle, testini yaz, PR aç. İlk katkı böyle olsun. |

---

## 8. Nerede ne var (hızlı harita)

| İhtiyaç | Yer |
|---|---|
| Permission/delegation kuralı | `internal/domain/permission.go` |
| Yetki orkestrasyonu (3 katman) | `internal/app/authorizer.go` |
| Use-case servisleri | `internal/app/*_service.go` |
| Dış dünya interface'leri | `internal/ports/` |
| SQL | `internal/postgres/*_repo.go` |
| HTTP handler + route + middleware | `internal/httpapi/` |
| OPA politikası | `deployments/opa/policies/authz.rego` |
| OpenFGA modeli | `internal/openfga/model.fga` |
| Şema + seed | `migrations/` |
| API sözleşmesi | `openapi.yaml` |
| Frontend yetki modeli | `web/src/auth/permissions.ts` |
| Dinamik arama (AND/OR + field-level) | `internal/domain/search.go`, `docs/search.md` |
| Derin dokümanlar | `docs/` |

Takıldığın her "neden böyle?" için önce `docs/architecture.md`'deki ADR'lere bak — büyük
ihtimalle cevabı orada, gerekçesiyle yazılı.
