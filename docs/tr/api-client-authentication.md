# API client kimlik doğrulaması

OAuth 2.0 client-credentials grant'ı üzerinden makineden-makineye erişim. API
client'lar bir kiracıya aittir ve asla onun dışında hareket etmez.

## Yaşam döngüsü

```
oluştur → { client_id, client_secret }   ← secret tam bir kez gösterilir
       → POST /oauth/token                → access token (30 dk)
       → API'yi Bearer <token> ile çağır
       → rotate-secret (eskisi anında ölür)
       → revoke (artık token yok)
```

## Oluşturma

`POST /api/v1/api-clients` şunu döner:

```json
{
  "data": {
    "client": { "client_id": "wg_OktfDAisD8acnhGc", "status": "active", "...": "..." },
    "client_secret": "k1nxNRqQ1_…",
    "warning": "Store this secret now. It cannot be retrieved again."
  }
}
```

Secret 32 rastgele bayttır, Argon2id hash olarak saklanır. Sonradan **geri
alınamaz**: `GET /api-clients/{id}` onu asla içermez ve veritabanı yalnızca
`$argon2id$v=19$m=65536,...` tutar. Kaybolan secret'lar kurtarılmaz, döndürülür.
Portal, secret'ı kapatmadan önce açık bir "bu secret'ı güvenle sakladım"
onayı gerektiren bir diyalogda gösterir.

## Token isteği

```bash
curl -X POST http://localhost:8090/api/v1/oauth/token \
  -H 'Content-Type: application/json' \
  -d '{"grant_type":"client_credentials","client_id":"wg_…","client_secret":"…"}'
```

`application/x-www-form-urlencoded` de çalışır (RFC 6749 stili). Cevap, platform
zarfı değil, çıplak OAuth şeklidir:

```json
{ "access_token": "eyJ…", "token_type": "Bearer", "expires_in": 1799, "scope": "wallet.read" }
```

Token `principal_type=api_client` ve `client_id` taşır ve **hiç** `session_id`
taşımaz — kullanıcı token'ları ile client token'larının aşağıda her yerde
birbirinden ayrıldığı yer budur.

Bir token verilmeden önceki kontroller: status `active` (suspended/expired/revoked
değil), `expires_at`'i geçmemiş, `allowed_ips` boş değilse kaynak IP listede,
secret sabit-zamanlı doğrulanıyor ve sahip kiracı operasyonel. Her sonuç
audit'lenir. Başarısızlıklar, client'ların sayılamaması için tek tip 401 döner.

## Yetkilendirme

Client'lar kendi izinlerini tutar, kullanıcılarla aynı delegasyon kuralıyla
verilir — sende olmayan yetkiyi bir client'a veremezsin. Platform-only izinler
ve `scope=all` doğrudan reddedilir. Grant'ı olmayan bir client başarıyla kimlik
doğrular ve hiçbir şey çağıramaz.

## Rotasyon ve iptal

`POST /api-clients/{id}/rotate-secret` yeni bir secret verir ve öncekini anında
geçersiz kılar (E2E suite'inde doğrulandı: eski → 401, yeni → 200).
`DELETE /api-clients/{id}` client'ı iptal eder; token istekleri sonra başarısız olur.

## Operasyon notları

* Secret'ı kendi secret manager'ında sakla, asla kaynak kodda.
* Bir programda ve erişimi olan biri ayrıldığında döndür.
* Çağıranın kararlı çıkış adresleri varsa `allowed_ips` kullan.
* Çalışan en dar scope'u ver — `tenant` yerine `own`/`assigned`.
* `last_used_at`, artık kullanılmayan ve iptal edilmesi gereken client'ları ortaya çıkarır.
