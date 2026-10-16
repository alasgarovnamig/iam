# Yerel geliştirme

## Ön koşullar

Docker Desktop (veya Docker Engine + compose v2), Go 1.25+, Node 20+.

## İlk çalıştırma

```bash
cp .env.example .env
docker compose up --build          # ilk seferde ~3–5 dakika
```

Portal http://localhost:3000 · API http://localhost:8090. `.env`'deki bootstrap
admin ile giriş yap; şifreni değiştirmen zorunlu tutulacak.

## Günlük çalışma

```bash
# Altyapı docker'da, API host'ta (hızlı rebuild, debugger-dostu)
docker compose up -d postgres redis kafka opa openfga-db openfga-migrate openfga
make migrate-up
make dev

# Portal, o API'ye karşı hot reload ile
cd web && npm run dev      # http://localhost:5173
```

`http://localhost:5173` zaten CORS allow-list'inde.

API'yi host'ta çalıştırmak app-rolü URL'sini gerektirir, çünkü compose'un değeri
`postgres` hostname'ini gösterir:

```bash
export DATABASE_APP_URL="postgres://wallet_app:wallet_app_dev_password@localhost:5432/wallet_gateway?sslmode=disable"
```

## Testler

```bash
make test               # Go birim
make opa-test           # Rego politikaları
make test-integration   # kara-kutu API — stack'in ayakta olması gerekir
make test-e2e           # Playwright — stack'in ayakta olması gerekir
cd web && npm test
```

Her iki entegrasyon suite'i de gerçek bir stack ile konuşur:

```bash
docker compose up --build -d
make test-integration
PORTAL_URL=http://localhost:3000 make test-e2e
```

Bilinçli olarak birkaç session paylaşırlar — login rate-limitli ve refresh
token'lar tek-kullanımlık, o yüzden her testte kimlik doğrulayan bir suite kendi
güvenlik kontrollerine takılırdı.

## Faydalı komutlar

```bash
docker compose ps                                   # ne sağlıklı
docker compose logs -f api                          # API'yi izle
docker compose exec postgres psql -U wallet -d wallet_gateway
docker compose exec redis redis-cli KEYS 'wallet-gateway:*'
docker compose exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list
curl -s localhost:8090/readyz | jq                  # bağımlılık sağlığı
```

İzin cache'ini ve invalidation'ını incele:

```bash
docker compose exec redis redis-cli GET "wallet-gateway:v1:tenant:<id>:permver"
docker compose exec redis redis-cli KEYS 'wallet-gateway:*:permissions'
```

Outbox'ın boşalmasını izle:

```bash
docker compose exec postgres psql -U wallet -d wallet_gateway \
  -c "SELECT status, count(*) FROM outbox_events GROUP BY 1;"
```

## Bir şeyleri değiştirme

**Yeni bir API endpoint'i** — domain kuralı → application use case (yetkilendir,
audit'le, event yay) → yeni bir bağımlılık gerekiyorsa port → adapter → handler →
route → OpenAPI → `make openapi` → testler.

**Yeni bir izin** — `migrations/0008_seed_catalog.up.sql`'e (ya da yeni bir
migration'a) ekle, onu tutması gereken system rollere iliştir ve kodu portal'ın
`auth/permissions.ts`'indeki `P`'ye ekle.

**OpenFGA modeli** — yalnızca `internal/openfga/model.fga`'yı düzenle;
gömülüdür ve boot'ta uygulanır. `go test ./...` transform'unu doğrular.

**Bir OPA politikası** — `deployments/opa/policies/authz.rego`'yu düzenle,
`deployments/opa/tests/`'e bir test ekle, `make opa-test` çalıştır. OPA
mount'lanmış dosyaları yeniden yükler, yani rebuild gerekmez.

## Sıfırlama

```bash
docker compose down -v      # tüm volume'ları düşürür: veritabanları, Kafka, Grafana
docker compose up --build
```

## Sorun giderme

| Belirti | Sebep |
|---|---|
| Test çalışması sırasında `429` | Rate limiter. Yerelde her şey tek IP paylaşır; `.env`'de `RATE_LIMIT_RPS`'i yükselt, `LOGIN_RATE_LIMIT_PER_MIN`'i sıkı tut. |
| Bir client'ta `TOKEN_REUSE_DETECTED` | Bir şey dönmüş bir refresh token'ı tekrar oynattı. Tasarım gereği çalışıyor — rotasyonu kapatarak "düzeltme". |
| `403 ACCOUNT_LOCKED` | Beş yanlış şifre. `POST /users/{id}/unlock`, ya da 15 dakika bekle. |
| Login CORS'la blokta | Portal origin'i `CORS_ALLOWED_ORIGINS`'te yok, ya da özel bir header allow-list'te değil. |
| `no transaction in context` | Bir repository `TxManager.WithTx` dışında çağrıldı — RLS bağlamı olmazdı. |
| `wallet_app` olarak boş sorgu sonuçları | Hiç tenant GUC set edilmedi. Bu, deny-by-default RLS işini yapıyor demektir. |
| API `openfga not ready` ile çıkıyor | OpenFGA boot'unu bitirmedi; compose retry eder, ya da önce onu başlat. |
