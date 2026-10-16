# Güvenlik

## Kontroller

| Kontrol | Uygulama |
|---|---|
| Varsayılan reddet | İzin yok → erişim yok. Bağlamsız RLS → satır yok. OPA `default allow := false`. |
| En az ayrıcalık | Field-level grant'lar; scope'lar; delegasyon kuralı herkesin dağıtabileceğini kısar. |
| Kiracı izolasyonu | Uygulama + OPA + PostgreSQL RLS (üç bağımsız katman). |
| Açık şifre yok | Argon2id (64 MB, t=3, p=2), PHC-kodlu. |
| Açık refresh token yok | Yalnızca SHA-256 hash; token opak rastgele bayt. |
| Açık API secret yok | Argon2id; tam bir kez gösterilir, sonra kurtarılamaz. |
| Sabit-zaman karşılaştırma | Argon2id doğrulayıcıda `subtle.ConstantTimeCompare`. |
| Güvenli rastgele | Token, secret ve client id için `crypto/rand`. |
| Şifre politikası | ≥10 karakter, büyük + küçük + rakam + sembol, ≤128. |
| Brute-force koruması | IP başına 10 giriş/dk (**fail-closed**); 5 başarısızlık → 15 dk kilit. |
| Rate limiting | IP başına istek limiter'ı; login ve OAuth için ayrı daha sıkı limiter. |
| Hesap kilitleme | `status = locked` + `locked_until`; doğru şifre yine 403 döner. |
| Refresh rotasyonu | Tek-kullanımlık token'lar, her refresh'te döner. |
| Yeniden-kullanım tespiti | Dönmüş bir token'ı tekrar oynatmak tüm aileyi iptal eder. |
| Secret rotasyonu | API client secret'ları döner; eskisi anında ölür. |
| Hassas alan redaksiyonu | Audit state'lerinde `RedactSensitive`; secret'ların hiç read izni yok. |
| SQL injection | Her yerde parametreli sorgular; sort/filter alanları allow-list'li, asla interpolasyon yok. |
| Mass assignment | Bilinmeyen gövde alanları → 400; korumalı alanlar `update.<field>` gerektirir → 403. |
| Açık DTO eşleme | Elle yazılmış DTO'lar; entity'lere otomatik struct binding yok. |
| CORS | Sıkı origin allow-list; credential'lar yalnızca listeli origin'ler için etkin. |
| Güvenlik header'ları | `nosniff`, `DENY`, `no-referrer`, `no-store`, kısıtlı CSP. |
| İstek boyutu limitleri | `MaxBytesReader` (varsayılan 1 MB). |
| Timeout'lar | İstek başına context timeout; sunucu read/write/idle timeout'ları. |
| Girdi doğrulama | Domain + `internal/validation`; UUID, e-posta ve slug kontrolleri. |
| Audit logging | Reddler ve yükseltme girişimleri dahil her güvenlik-hassas action. |
| Loglarda secret yok | Yapısal loglar id ve neden taşır, asla credential. |
| Yükseltme önleme | Delegasyon kuralı, platform-only korumaları, `scope=all` korumaları, değiştirilemez system roller. |

## OWASP API Security Top 10

| Risk | Nasıl ele alınıyor |
|---|---|
| **API1 Bozuk nesne seviyesi yetki** | Her nesne kendi kiracısına/sahibine karşı yetkilendirilir, artı RLS. Başka kiracının kullanıcısını tam id'siyle okumak 404 döner — doğrulandı. |
| **API2 Bozuk kimlik doğrulama** | Argon2id, kilitleme, rate limiting, yeniden-kullanım tespitli dönen tek-kullanımlık refresh token'lar, HttpOnly cookie'ler, tam JWT claim doğrulaması. |
| **API3 Bozuk nesne özelliği seviyesi yetki** | Field-level read/update/search izinleri. Okunamayan alanlar çıkarılır; yazılamayan alanlar isteği reddeder; bilinmeyen alanlar reddedilir. |
| **API4 Sınırsız kaynak tüketimi** | Rate limit'ler, `page_size` 100'de sınırlı, gövde boyutu limiti, istek timeout'ları, connection pool sınırları. |
| **API5 Bozuk fonksiyon seviyesi yetki** | İzinler `resource.action`, asla route adı. OPA + domain motoru ikisi de kontrol eder; platform-only action'lar tenant principal'lerine ulaşamaz. |
| **API6 Hassas iş akışlarına sınırsız erişim** | Yazmalarda idempotency anahtarları; optimistic locking; tekrarlı başarısızlıkta kilit; her akış audit'li. |
| **API7 SSRF** | Servis kullanıcı-kontrollü dış istek yapmaz; OPA/OpenFGA/Kafka endpoint'leri yalnızca config'ten. |
| **API8 Güvenlik yanlış yapılandırması** | Deny-by-default RLS, sıkı CORS, güvenlik header'ları, app rolü RLS'i bypass edemez, ≥32 karakter imza anahtarı olmadan başlamaz. |
| **API9 Uygunsuz envanter yönetimi** | TypeScript client'ı üreten tek versiyonlu OpenAPI dokümanı; `/healthz`, `/readyz`, `/metrics` açık. |
| **API10 Üçüncü-taraf API'lerin güvensiz tüketimi** | OPA/OpenFGA cevapları doğrulanır ve fail-closed olur; her çağrıda timeout. |

## Tehditler ve cevaplar

**Çalınan refresh token** — bir kez kullanılabilir; taraflardan birinin sonraki
kullanımı aileyi iptal eder ve `refresh_token_reuse_detected` audit'ler.

**Portalda XSS** — refresh token HttpOnly ve okunamaz; access token bellekte ve
15 dakikada sona erer; web depolamada hiçbir şey yok.

**`/auth/refresh`'e CSRF** — `SameSite=Strict`, cross-site isteklerin cookie
taşımaması demektir; endpoint'in kendisinin de o ötesinde bir örtük yetkiye
ihtiyacı yok.

**Yetki yükselten kötü niyetli tenant admin** — delegasyon kuralı grant'ları
tuttuklarıyla sınırlar; platform-only izinler ve `scope=all` reddedilir; system
roller değiştirilemez; her girişim transaction rollback olsa bile audit'lenir.

**Ele geçirilmiş app veritabanı credential'ı** — app rolü RLS'i bypass edemez,
yani sorgular yine bir kiracı bağlamı gerektirir; audit satırlarını da UPDATE
veya DELETE edemez.

**İzlerini silen içerideki biri** — `audit_logs`, UPDATE/DELETE'i bir trigger'la
bloklar ve app rolü yalnızca INSERT tutar.

## Production kontrol listesi

- [ ] Secret'lar bir secret manager'dan; `.env` asla commit edilmez.
- [ ] `JWT_SIGNING_KEY` ≥ 32 karakter entropi; örtüşme penceresiyle döndür.
- [ ] TLS her yerde; refresh cookie'nin `Secure` taşıdığını doğrula (`X-Forwarded-Proto: https` arkasında).
- [ ] App `wallet_app` ile bağlanır, tablo sahibiyle **değil** — aksi halde RLS sessizce hiçbir şey yapmaz.
- [ ] `OPENFGA_STORE_ID` ve `OPENFGA_MODEL_ID` sabitlenmiş.
- [ ] `RATE_LIMIT_RPS` gerçek client IP'lerine göre ayarlı; `LOGIN_RATE_LIMIT_PER_MIN` sıkı tutulmuş.
- [ ] Kafka TLS/SASL ile; otomatik topic yaratma kapalı.
- [ ] Bootstrap admin şifresi döndürülmüş; kişi-başı platform hesapları açılmış.
- [ ] Audit saklama, partition ve arşivleme planlanmış.
- [ ] `authz_decisions_total{decision="deny"}` sıçramalarına, kilit oranlarına, outbox gecikmesine, DLQ derinliğine alarm.
- [ ] `govulncheck`, `gosec`, `npm audit`, Trivy CI'da (hazır) ve incelenmiş.

## Raporlama

Şüpheli açıkları platform güvenlik ekibine özel olarak bildirin. Halka açık bir
issue açmayın.
