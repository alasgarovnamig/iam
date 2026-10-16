# Kimlik doğrulama

İki akış: etkileşimli kullanıcılar ve makine istemcileri. İkisi de yalnızca
kimlik taşıyan JWT access token verir — izinler istek başına çözülür (ADR-002).

## Access token'lar

| Claim | Anlamı |
|---|---|
| `sub` | Principal id (kullanıcı id'si veya API client id'si) |
| `principal_type` | `platform_user` · `tenant_user` · `api_client` · `service_account` · `system_service` |
| `tenant_id` | Platform principal'leri için boş |
| `session_id` | Yalnızca kullanıcı session'ları |
| `client_id` | Yalnızca API client'lar |
| `jti`, `iss`, `aud`, `iat`, `nbf`, `exp` | Standart claim'ler; hepsi doğrulanır |

HS256, TTL 15 dakika (client'lar için 30). Doğrulama algoritmayı, issuer'ı,
audience'ı ve zaman claim'lerini zorlar — `alg: none` veya RS/HS karışımı bir
token reddedilir.

## Kullanıcı girişi

```
POST /api/v1/auth/login { email, password, tenant_slug? }
```

`tenant_slug` yalnızca aynı e-posta birden çok kiracıda varken gerekir;
platform hesabı aksi halde kazanır.

* **Rate limit'li** — IP başına 10/dk ve **fail-closed**: Redis sayamıyorsa giriş
  korumasız bırakılmak yerine reddedilir.
* **Argon2id** doğrulama (64 MB, t=3, p=2), sabit-zamanlı.
* **Kilitleme** — 5 ardışık başarısız girişim hesabı 15 dakika kilitler ve
  `status = locked` yapar. Doğru şifre o zaman yine `403 ACCOUNT_LOCKED` döner.
* **Tek tip hatalar** — bilinmeyen e-posta ile yanlış şifre ayırt edilemez
  (`INVALID_CREDENTIALS`, "E-posta veya şifre hatalı.").
* **Audit'li** — başarı, başarısızlık ve red, nedeniyle.

Başarısız-girişim sayacı, kilit ve audit satırı, **istek reddedilmesine rağmen
commit edilir**; bunun neden söylenmesi gerektiği için bkz. ADR-005.

## Refresh rotasyonu ve yeniden-kullanım tespiti

Refresh token'lar opak (32 rastgele bayt), yalnızca SHA-256 hash olarak saklanır,
tek kullanımlık ve girişte yaratılan bir *aileye* aittir.

```
refresh(token):
  hash ile session bul                        → bilinmiyor? 401
  if session.status != active:                # zaten dönmüş veya iptal edilmiş
      TÜM aileyi iptal et                      # commit edilir
      audit(denied, refresh_token_reuse_detected)
      return 401 TOKEN_REUSE_DETECTED
  if expired: 401
  session.status = rotated
  aynı ailede yeni session ver                 # yeni refresh token
```

Yani çalınan bir token en fazla bir kez çalışır ve hırsız ya da kurban tekrar
kullandığı anda ikisi de dışarı atılır ve olay kaydedilir. Kaydedilmiş cookie'yi
tekrar oynatan bir test suite'inin tespiti tetiklemesinin nedeni de budur —
kontrolün çalışması, bir bug değil.

## Tarayıcıda token yönetimi

Tarayıcılar `X-Auth-Mode: cookie` gönderir:

* refresh token **yalnızca** `wg_refresh_token` olarak döner — `HttpOnly`,
  `SameSite=Strict`, `Path=/api/v1/auth`, TLS arkasında `Secure` — ve bir cevap
  gövdesinde asla görünmez;
* access token bir JavaScript değişkeninde yaşar, asla `localStorage`/`sessionStorage`'da;
* sayfa yenileme session'ı cookie ile `/auth/refresh` çağırarak geri yükler;
* bir 401, tek-uçuş bir refresh ve orijinal isteğin bir kez tekrar denenmesini tetikler.

XSS bu yüzden refresh token'ı okuyamaz ve `SameSite=Strict` refresh endpoint'inin
CSRF yüzeyini kapatır. Makine istemcileri header'ı atlar ve token'ı gövdede alır.

## Şifre politikası ve değişimi

En az 10 karakter; büyük harf, küçük harf, rakam ve sembol; en fazla 128.
Sunucu tarafında zorlanır (`internal/auth`) ve portalda aynı kurallarla
yansıtılır.

`POST /auth/change-password` mevcut şifreyi doğrular, hash'i döndürür,
`must_change_password`'ü temizler ve **diğer tüm session'ları** iptal eder —
şifre değişimi birini kovmanın yoludur.

Her kullanıcı, bootstrap admin dahil, `must_change_password=true` ile başlar;
portal bitene kadar tüm uygulamayı bloklar.

## Şifre sıfırlama (genişleme noktası)

Uygulanmadı. İhtiyacı olan yapı zaten burada: `Session` aileleri,
`password_changed_at`'li `Credential`, audit action'ları ve outbox event'leri.
Bir reset servisi tek-kullanımlık bir token tablosu ekler, son adım için
`AuthService.ChangePassword`'ü yeniden kullanır ve `user.password_reset.v1`
yayınlar. Mevcut tablolara şema değişikliği gerekmez.

## Session'lar

`GET /auth/sessions` aktif session'larını listeler (cihaz, IP, ilk ve son
kullanım, hangisinin güncel olduğu). `DELETE /auth/sessions/{id}` birini iptal
eder — yalnızca kendininkini. `POST /auth/logout-all` hepsini iptal eder.

Session'lar ayrıca şu durumlarda otomatik iptal edilir: şifre değişimi,
suspend/disable, silme ve refresh-token yeniden kullanımı.
