# Wallet Gateway — Platform (Türkçe)

Müşteri şirketlerin, onlarca üçüncü-taraf sağlayıcıyla tek tek entegre olmak
yerine tek sözleşmeyle bağlandığı ve tüm hizmetleri tek API üzerinden aldığı bir
ağ geçidi (gateway). İleride cüzdan yönetimi, ödemeler, kartlar, banka
entegrasyonları, KYC/KYB, işlemler, payout/pay-in, iade, mutabakat, limitler,
ücretler, bildirimler, raporlama ve sağlayıcı yönlendirmesini kapsayacak.

**Bu repo şu an o hizmetlerin üzerinde duracağı temeli** production'a yakın
şekilde içeriyor:

| Teslim edilen | Not |
|---|---|
| User Management servisi | Tenant, kullanıcı, rol, izin, API client, audit |
| Kimlik doğrulama | E-posta/şifre + OAuth 2.0 client credentials, dönen refresh token |
| Çok-kiracılık | Uygulama filtreleme **ve** PostgreSQL Row-Level Security |
| Yetkilendirme | OpenFGA ilişkileri + OPA politikası + field-level izin motoru |
| Dinamik arama | AND/OR/operatör koşul ağaçları + alan-bazlı `search.<field>` yetkisi |
| İzin delegasyonu | Yalnızca kendinde olanı, kendi scope'undan dar/eşit verebilirsin |
| Audit log | Ekle-only, veritabanı seviyesinde değiştirilemez |
| React admin portal | İzin-farkında UI, HttpOnly refresh cookie |
| Altyapı | PostgreSQL, Redis, Kafka, OPA, OpenFGA, Prometheus, Grafana, Jaeger |

Cüzdan/işlem/ödeme/sağlayıcı servisleri **henüz yok**. Bu repo tek bir servis ve
düz kurulu: tek Go modülü, tek binary seti, tek frontend.

---

## Hızlı başlangıç

```bash
git clone <repo-url> && cd wallet-gateway
cp .env.example .env          # yerel varsayılanlar; gerçek .env'i asla commit etme
docker compose up --build     # ilk çalıştırma imajları çeker + derler, ~3-5 dk
```

Sonra portalı **http://localhost:3000** aç ve `.env`'deki bootstrap admin ile
giriş yap.

| Servis | URL |
|---|---|
| Admin portal | http://localhost:3000 |
| API | http://localhost:8090 |
| API sağlık / hazırlık | http://localhost:8090/healthz · `/readyz` |
| API metrikleri | http://localhost:9091/metrics |
| Kafka UI | http://localhost:8085 |
| OpenFGA playground | http://localhost:3010/playground |
| OPA görüntüleyici (politika, veri, canlı karar testi) | http://localhost:3020 |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3001 (admin/admin) |
| Jaeger | http://localhost:16686 |

`docker compose up`: migration'ları uygular, izin kataloğu + system rolleri
seed'ler, bootstrap admin'i yaratır ve OpenFGA modelini kaydeder — hepsi
idempotent, yeniden başlatmak güvenli.

### ⚠️ Bootstrap admin credential'ları

Bootstrap admin ortam değişkeninden yaratılır ve **yalnızca** yerel geliştirme /
ilk açılış içindir:

```env
BOOTSTRAP_ADMIN_EMAIL=sr.alasgarov@gmail.com
BOOTSTRAP_ADMIN_PASSWORD=Anh0@111118
```

* Şifre **asla** kaynak koda gömülü değildir — bu değişkenler olmadan servis başlamaz.
* Hesap `must_change_password=true` ile yaratılır; portal, başka hiçbir şeye
  erişmeden önce şifre değişimini zorlar.
* Seed idempotent: tekrar çalıştırmada hesabı ve Super Admin rolünü doğrular,
  mevcut şifreyi asla ezmez.
* **Production'da** bunları bir secret manager'dan verin, benzersiz yüksek-entropili
  bir şifre kullanın, ilk girişten sonra hemen değiştirin ve bootstrap'ı paylaşmak
  yerine kişi-başı platform hesabı açın. `.env`'i commit etmeyin.

---

## Mimari

Hexagonal (ports & adapters) + DDD + Clean Architecture. Bağımlılık oku **içeri**
bakar: domain, PostgreSQL/Redis/Kafka/OPA/OpenFGA/HTTP/JWT'nin hiçbirini bilmez.

```
internal/httpapi   →   internal/app   →   internal/domain
  (HTTP, JSON)          (use-case)          (saf kural)
                                                 ▲
              internal/ports ── implement eden ──┘
              internal/postgres · cache · kafka · opa · openfga
```

## Klasör yapısı

Tek Go modülü. Her paket `internal/` altında **tam bir seviye** durur ve adı ne
olduğunu söyler.

```
wallet-gateway/
├── cmd/{api,worker,migrate}/    # üç binary
├── internal/
│   ├── domain/                  # aggregate'ler, izin motoru, arama modeli
│   ├── ports/                   # çekirdeğin dünyadan istediği interface'ler
│   ├── app/                     # use-case'ler, authorizer, outbox worker
│   ├── postgres/                # SQL repo'ları, RLS transaction'ları
│   ├── httpapi/                 # chi route, handler, DTO, middleware
│   ├── cache/                   # Redis: izin, idempotency, rate limit
│   ├── kafka/                   # outbox yayıncısı
│   ├── opa/ · openfga/          # politika kararı · ilişki kontrolü
│   ├── setup/                   # config + bağımlılık wiring
│   ├── seed/                    # idempotent bootstrap (admin, FGA tuple)
│   └── auth/ apperr/ db/ events/ idempotency/ observability/ validation/
├── web/                         # React + TypeScript + Vite + MUI
├── tests/integration/           # kara-kutu API testleri
├── migrations/ · deployments/ · docs/
├── openapi.yaml · docker-compose.yml · Makefile
```

**Bir isteğin izini sürmek** — her yol aynı dört durak:

```
internal/httpapi/router.go        hangi URL → hangi handler
internal/httpapi/handlers_*.go    parse et, tek bir use-case çağır
internal/app/<şey>_service.go     yetkilendir → transaction → audit → event
internal/postgres/<şey>_repo.go   SQL
```

Ayrıntılı Türkçe dokümanlar bu klasörde:

| Belge | İçerik |
|---|---|
| [onboarding.md](onboarding.md) | **Buradan başla** — yeni geliştirici nasıl okumalı, anlamalı, geliştirmeli |
| [architecture.md](architecture.md) | Katmanlar, sınırlar ve her önemli karar için ADR'ler |
| [domain-model.md](domain-model.md) | Aggregate'ler, entity'ler, durum makineleri |
| [authorization.md](authorization.md) | Üç yetkilendirme katmanı ve bir kararın nasıl verildiği |
| [permission-delegation.md](permission-delegation.md) | Delegasyon kuralı ve algoritması |
| [search.md](search.md) | Dinamik arama: koşul ağacı, alan-bazlı arama yetkisi, SQL güvenliği |
| [authentication.md](authentication.md) | Giriş, refresh rotasyonu, yeniden kullanım tespiti, kilitleme |
| [api-client-authentication.md](api-client-authentication.md) | Client-credentials akışı ve secret yönetimi |
| [multi-tenancy.md](multi-tenancy.md) | Kiracı izolasyonu ve Row-Level Security |
| [events.md](events.md) | Event zarfı, katalog ve transactional outbox |
| [security.md](security.md) | Kontroller, OWASP API Top 10 eşlemesi, production notları |
| [local-development.md](local-development.md) | Günlük iş akışları |
| [sequence-diagrams.md](sequence-diagrams.md) | Ana akışlar için Mermaid diyagramları |

> İngilizce orijinaller bir üst klasörde ([`../`](../)). Bu klasör onların
> Türkçe karşılığıdır; içerik aynıdır, kod referansları birebir aynı dosyaları
> gösterir.

---

## Testlerin çalıştırılması

```bash
make test              # Go birim testleri
make opa-test          # OPA politika testleri
make test-integration  # kara-kutu API testleri — stack ayakta olmalı
make test-e2e          # Playwright — stack ayakta olmalı
cd web && npm test   # React/TS birim testleri
```

## Production güvenlik notları (özet)

- **Secret'lar** secret manager'dan; `.env` asla commit edilmez.
- **TLS her yerde** — refresh cookie HTTPS arkasında `Secure` alır.
- **DB rolü** — uygulama `wallet_app` ile bağlanmalı, tablo sahibi olarak değil:
  RLS tablo sahibine uygulanmaz, izolasyonu veritabanı garantisi yapan budur.
- **OpenFGA'yı sabitle** — `OPENFGA_STORE_ID`/`OPENFGA_MODEL_ID` production'da pinlensin.
- **Rate limit'ler** — dev'de tek IP paylaşıldığı için gevşek; ortama göre ayarlayın,
  `LOGIN_RATE_LIMIT_PER_MIN` sıkı kalsın.
- **Audit** — `audit_logs` ekle-only (trigger UPDATE/DELETE'i engeller); büyümeden
  önce partition/arşiv planlayın.
