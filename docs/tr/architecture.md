# Mimari

## Amaç

Müşteri şirketlerin onlarca sağlayıcı yerine tek bir ağ geçidiyle entegre olduğu
bir sistem. Buradaki her şey **temel**: parayı taşıyacak servisler (cüzdan,
ödeme, işlem, sağlayıcı yönlendirmesi) henüz yok; bu repo'nun işi, onları
eklemeyi ucuz ve güvenli kılmaktır.

Bu, aşağıdaki her kararı şekillendiriyor. Kimlik, kiracılık, yetki ve audit —
sonradan geri takılamayacak parçalardır. Daha zayıf bir kiracı sınırı varsayan
bir ödeme servisi, genişletilmez; yeniden yazılırdı.

## Katmanlar

Hexagonal mimari; bağımlılık oku içeri bakar.

```
gelen adapter (httpapi)
        │  DTO gir, DTO çık. İş kuralı yok.
        ▼
application (use-case'ler)
        │  Transaction, yetki orkestrasyonu, audit, outbox, cache.
        ▼
domain
        │  Aggregate'ler, izin motoru, delegasyon kuralı.
        │  Standart kütüphane + errors paketi dışında hiçbir şey import etmez.
        ▼
ports (interface) ◄──── implement eden ──── giden adapter'lar
                                             postgres · redis · kafka
                                             openfga · opa
```

Somut olarak ne nerede yaşar:

| Paket | İçerir | Asla içermez |
|---|---|---|
| `internal/domain/` | İzin cebiri, delegasyon kuralı, durum makineleri, invariant'lar | SQL, HTTP, JWT, vendor SDK |
| `internal/ports/` | Çekirdeğin dünyadan ihtiyaç duyduğu interface'ler | Implementasyon |
| `internal/app/` | Use-case'ler, transaction sınırları, authorizer, outbox worker | SQL string'i, `http.Request`, chi, pgx |
| `internal/postgres/`, `internal/httpapi/`, `internal/cache/`, `internal/kafka/`, `internal/opa/`, `internal/openfga/` | pgx sorguları, chi handler'ları, Redis anahtarları, Kafka writer'ları, OPA/OpenFGA HTTP | İş kuralı |
| `internal/setup/` | Config parse, bağımlılık wiring | İş kuralı |

`internal/` altındaki geri kalan her şey küçük ve adı kendini anlatır: `auth`
(JWT + Argon2id), `apperr` (API'nin konuştuğu tek hata tipi), `db` (pool),
`events` (zarf), `idempotency`, `observability`, `validation`, `seed`.

`adapters/` diye bir gruplama katmanı **yok**. Postgres ile konuşan pakete
`postgres` denir; gruplama, bilgi eklemeden bir klasör ekliyordu.

Bağımlılık kuralını dürüst tutan test: `internal/domain` üçüncü-taraf import
olmadan derlenir ve testleri hiç fake gerektirmez — izin kümelerini doğrudan
kurarlar.

## Neden tek düz modül

Bu repo önce monorepo olarak kuruluydu (`apps/<servis>/internal/...` artı ortak
bir `packages/go/` kütüphanesi). Bu şeklin her okumada bir bedeli vardı: ilk
gerçek dosyaya inmek için beş klasör seviyesi, satıra sığmayan import yolları ve
tek tüketicisi yine o tek servis olan bir "ortak kütüphane".

Ortada tek bir servis var, o yüzden tek bir modül var. İkinci bir servis
gerçekten gelirse kendi repo'sunu alır ve ihtiyacı olanı import eder; bir paketi
sonradan ayırmak bir yeniden adlandırmadır, ucuzdur. İkinci tüketici ortada
yokken yeniden kullanımı tahmin etmek ucuz değildir.

---

# Mimari Karar Kayıtları (ADR)

## ADR-001: Yetkiyi OpenFGA, OPA ve domain motoru arasında bölmek

**Durum:** kabul edildi

**Bağlam.** Gereksinimler tek kelimeye sığmış üç farklı sorudur. "Bu kullanıcı o
kiracının üyesi mi?" bir graf sorusudur. "Bu istek şu an, bu IP'den, bu path'te
izinli mi?" bir politika sorusudur. "Bu cevabın hangi alanlarını görebilir?"
kendi izin kataloğumuza bağlı bir veri sorusudur.

**Karar.** Her soruyu iyi cevaplayan araca ver:

* **OpenFGA** yalnızca ilişki/sahiplik grafını tutar — üyelik, rol atama, kaynak
  sahipliği. Asla izin semantiği değil.
* **OPA** istek-bağlamı politikasını tutar: kiracı sınırı, scope/effect ile izin
  eşleştirme, field-level çıktı, satır filtreleri.
* **Domain motoru** action-level izni Go'da yeniden değerlendirir ve field
  görünürlüğünü hesaplar.

**Sonuçlar.** Yetkili istek başına iki ağ çağrısı (ikisi de cache'li veya hızlı;
OPA bir sidecar). Karşılığında ilişki verisi politikaya sızmaz, politika izole
test edilebilir (`make opa-test`), domain kuralları hiç altyapı olmadan birim-test
edilir. Domain'in yeniden kontrolü bilinçli fazlalıktır: OPA ile motor bir gün
anlaşmazsa istek reddedilir.

**Reddedilen:** her şeyi OpenFGA'ya koymak (field-level/condition'ı temiz
ifade edemez); her şeyi OPA'ya (Rego'da graf yavaş/hantal); kendimiz yazmak
(en az elle yazmak isteyeceğin parça).

## ADR-002: İzinler istek başına çözülür, JWT'ye konmaz

**Durum:** kabul edildi

**Karar.** JWT yalnızca kimlik taşır: `sub`, `principal_type`, `tenant_id`,
`session_id`, `client_id`, `jti`, `iss`, `aud`, `iat`, `nbf`, `exp`. İzinler her
istekte Redis'ten (yoksa PostgreSQL'den) çözülür.

**Sonuçlar.** İptal edilen bir izin, access-token TTL'sinin sonunda değil,
**anında** etki eder — bir finansal platform için, kovulmuş bir çalışanın 15
dakika daha erişiminin sürmesi kabul edilemez. Bedeli istek başına bir aramadır;
bu yüzden effective izin kümesi versiyonlu tenant anahtarıyla cache'lenir.

## ADR-003: Kiracı izolasyonu yalnızca kodla değil, PostgreSQL'le

**Durum:** kabul edildi

**Bağlam.** Uygulama seviyesi `WHERE tenant_id = ?` unutulan tek bir cümle uzağında
cross-tenant sızıntıdır — ve bu, bir denetçinin tam olarak soracağı hata sınıfıdır.

**Karar.** Her kiracı-scope'lu tabloda Row-Level Security. Uygulama, tabloların
**sahibi olmayan** `wallet_app` rolüyle bağlanır, dolayısıyla RLS ona uygulanır.
Her iş birimi, transaction içinde `SET LOCAL` ile `app.tenant_id` (veya
`app.is_platform`) ayarlar. Hiçbir GUC set edilmezse hiçbir şey görünmez:
varsayılan olarak reddet.

**Sonuçlar.** Her repo çağrısı `TxManager.WithTx` içinde çalışmalı — repo aksi
halde çalışmayı reddeder, çünkü transaction dışı sorgu RLS bağlamı taşımaz.
Migration'lar sahip rolünü kullanmaya devam eder. Kazanç: Go'da eksik bir filtre
bir bug'dır, bir ihlal değil.

**Doğrulandı:** tenant GUC'si A'ya set edilmişken, B'nin kullanıcısını tam
id'siyle seçmek sıfır satır döner.

## ADR-004: Yetki fail-closed, cache fail-soft

**Durum:** kabul edildi

**Karar.** OPA veya OpenFGA erişilemezse → 503 ile reddet. Redis erişilemezse →
cache okumaları miss say, PostgreSQL'den cevapla — tek istisna, sayamadığında
girişi reddeden login limiter.

**Sonuçlar.** Bir OPA kesintisi güvenlik değil erişilebilirlik olayıdır. Bir
Redis kesintisi doğruluğa değil gecikmeye mal olur. Redis hatasının ölümcül
olduğu tek yer, fail-open olmanın daha kötü olacağı brute-force korumasıdır.

## ADR-005: İş reddi side-effect'lerini commit etmeli

**Durum:** kabul edildi

**Bağlam.** Doğal Go biçimi — transaction closure'ından hata dönmek — bir reddin
ürettiği kanıtı sessizce geri alıyordu. Başarısız login sayaçları kalıcı olmadı
(hesap hiç kilitlenmedi), refresh-token ailesi iptali geri alındı (çalınan token
zinciri kullanılabilir kaldı) ve yetki-yükseltme girişimleri audit'ten kayboldu.
Üçü de kod incelemesinde doğru görünüyordu ve üçü de uçtan uca test edildiğinde
bozuktu.

**Karar.** İş reddi closure'dan bir `outcome` değişkeniyle çıkar ve commit'ten
**sonra** döndürülür; yalnızca altyapı hataları closure'dan döner. Rollback'i
izleyen reddler kendi transaction'ında audit'lenir (`auditDelegationFailure`).

**Sonuçlar.** Biraz sıra dışı bir biçim, o yüzden her yerinde yorumlandı ve fake
transaction manager'ı closure hatalarında rollback yapan testlerle pinlendi —
deseni geri getirmek suite'i gürültülü şekilde patlatır.

## ADR-006: OpenFGA modeli, servisin boot'ta uyguladığı bir DSL dosyası

**Durum:** kabul edildi

**Karar.** `model.fga` OpenFGA adapter'ında yaşar, `go:embed` ile gömülür ve
boot'ta dönüştürülüp uygulanır. Store isimle bulunur veya yaratılır.
`OPENFGA_STORE_ID`/`OPENFGA_MODEL_ID` keşfi ezer.

**Sonuçlar.** Tek doğruluk kaynağı ve temiz bir OpenFGA'da kendini iyileştiren
bir boot. Her yeniden başlatmada yeni model versiyonu birikmesini önlemek için,
saklanan model ile bizimki DSL'e geri render edilip karşılaştırılır — bayt
karşılaştırması işe yaramaz çünkü OpenFGA sakladığını normalize eder.

## ADR-007: Refresh token'lar döner; yeniden kullanım aileyi iptal eder

**Durum:** kabul edildi

**Karar.** Refresh token'lar opak, SHA-256 hash olarak saklanır, tek kullanımlık
ve her refresh'te döner. Dönmüş bir token'ı sunmak tüm aileyi iptal eder.

**Sonuçlar.** Çalınan token en fazla bir kez kullanılır ve hırsızlık, taraflardan
biri tekrar kullandığı anda tespit edilir. Ödün gerçektir ve testlerde görünür:
token'ı tekrar oynatan her istemci — kaydedilmiş cookie'yi yeniden kullanan
naif bir test suite dahil — tespiti tetikler. Bu, kontrolün çalışmasıdır.

## ADR-008: Tarayıcı refresh token'ı asla görmez

**Durum:** kabul edildi

**Karar.** İstemciler `X-Auth-Mode: cookie` ile katılır; refresh token o zaman
yalnızca `/api/v1/auth`'a scope'lu HttpOnly, SameSite=Strict cookie'de gider ve
cevap gövdesinden çıkarılır. Access token yalnızca bellekte yaşar.

**Sonuçlar.** XSS refresh token'ı okuyamaz, SameSite=Strict refresh endpoint'inin
CSRF yüzeyini kapatır. `localStorage`'da hiçbir şey yok. Sayfa yenileme, session'ı
cookie ile `/auth/refresh` çağırarak geri yükler. Özel header'lar API'nin CORS
allow-list'inde `X-Auth-Mode`'u gerektirir — orada.

## ADR-009: Field izinleri açık grant'tir, action'dan ima edilmez

**Durum:** kabul edildi

**Karar.** `users.read` temel alanları verir; her korumalı alan `users.read.<field>`
gerektirir. Arama yine ayrıdır: `read.email` olsa bile e-postayla filtrelemek
için `users.search.email` gerekir. Okunamayan alanlar cevaptan çıkarılır;
yazılamayan alanları taşıyan istek gövdesi tüm isteği reddeder.

**Sonuçlar.** Daha çok katalog kaydı (56) ve bir müşterinin adını görebilen bir
destek elemanı, kullanıcı tabanını gerçekten e-postayla sayamaz — okumak ve
aramak, farklı riskleri olan farklı yeteneklerdir.

## ADR-010: Her event için transactional outbox

**Durum:** kabul edildi

**Karar.** İş transaction'ları `outbox_events`'e ekler; bir worker exponential
backoff ve dead-letter ile Kafka'ya yayınlar.

**Sonuçlar.** Rollback olan bir transaction için event yayınlanmaz ve Kafka
kapalı diye hiçbir state değişimi kaybolmaz. Teslimat en-az-bir-kez, o yüzden
consumer'lar idempotent olmalı — zarf tam bunun için `event_id` taşır.

## ADR-011: Değişebilir aggregate'lerde optimistic locking

**Durum:** kabul edildi

**Karar.** Tenant, kullanıcı, rol, API client'ta `version`. Güncellemeler
beklenen versiyonu taşır; uyumsuzluk `409 VERSION_CONFLICT` döner.

**Sonuçlar.** Eşzamanlı düzenlemeler, kullanıcının çözebileceği bir çakışma
olarak yüzeye çıkar; bir izin kümesinde sessiz son-yazan-kazanır, birinin az önce
kaldırdığı erişimi sessizce geri getirmek demek olurdu.

## ADR-012: Routing için chi

**Durum:** kabul edildi

**Karar.** HTTP adapter'ında `go-chi/chi`.

**Sonuçlar.** Standart `http.Handler` semantiği, middleware sıradan Go'dur ve
framework adapter'ın ötesine uzanmaz. Değiştirmek yalnızca `internal/httpapi`'ye
dokunur.

## ADR-013: Şifreler ve client secret'lar için Argon2id

**Durum:** kabul edildi

**Karar.** Argon2id (64 MB, 3 iterasyon, parallelism 2), PHC-kodlu hash;
karşılaştırma sabit-zamanlı. Client secret'lar aynı şekilde hash'lenir ve tam
bir kez gösterilir.

**Sonuçlar.** Login tasarım gereği ~50 ms CPU'ya mal olur. Testler varsayılanı
zayıflatmak yerine ucuz parametreleri açıkça kullanır.
