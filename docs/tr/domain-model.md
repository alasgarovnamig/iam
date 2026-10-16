# Domain modeli

`internal/domain` içinde saf Go — SQL, HTTP, JWT
veya vendor SDK yok. Aşağıdaki her şey altyapı olmadan birim-test edilebilir.

## Aggregate'ler ve entity'ler

| Tip | Rol |
|---|---|
| **Platform** | Örtük singleton (OpenFGA'da `platform:platform`); platform kullanıcılarının `tenant_id = NULL` |
| **Tenant** | Bir müşteri şirket için aggregate kök |
| **OrganizationUnit** | Bir kiracı içindeki ağaç; `team`/`organization`/`descendants` scope'larını destekler |
| **User** | Aggregate kök; `platform_user`, `tenant_user` veya `service_account` |
| **Credential** | Şifre hash'i ve kilit durumu; `User`'dan ayrı, böylece secret'lar bir DTO'ya iliştirilmez |
| **Session** | Bir refresh-token ailesi üyesi |
| **Role** | System (global, değiştirilemez) veya tenant custom |
| **PermissionDefinition** | Katalog kaydı: service/resource/action/field |
| **RolePermission** | Bir role iliştirilmiş tanım + scope + effect + conditions |
| **UserRole** | Rol ataması |
| **UserPermission** | Doğrudan grant, opsiyonel süreli |
| **APIClient** | Hash'lenmiş secret'ı olan makine principal'i |
| **APIClientPermission** | Bir client'a grant |
| **AuditLog** | Değiştirilemez güvenlik kaydı |
| **OutboxEvent** | Yayın bekleyen event |

Değişebilir entity'lerde ortak alanlar: `id` (UUID), `created_at`, `updated_at`,
`created_by`, `updated_by`, `version` (optimistic locking).

## Value object'ler

**Permission** — `service.resource.action[.field]` + `scope` + `effect` +
`conditions`; `Code()` kanonik string'i render eder. **PermissionSet**, `Allows`,
`AllowsField`, `AllowedFields`, `MaxScopeFor` ve `CanDelegate`'i yanıtlar —
delegasyon kuralı burada yaşar.

**Principal** — kimliği doğrulanmış çağıran: id, kind, tenant, session/client id.

## Durum makineleri

```
User:    pending ──▶ active ──▶ suspended ──▶ active
                 └──▶ disabled ◀── (herhangi)
         active ──▶ locked ──▶ active            (5 başarısız giriş → locked)
         disabled sonlanmış

Tenant:  pending ──▶ active ──▶ suspended ──▶ active
                 └──▶ terminated ◀── (herhangi)   (sonlanmış)

APIClient: active ⇄ suspended
           active ──▶ expired | revoked          (sonlanmış)

Session: active ──▶ rotated ──▶ reused           (hırsızlık tespit edildi)
                 └──▶ revoked
```

Geçişler `TransitionTo`'dan geçer; haritada olmayan her şeyi `409 CONFLICT` ile
reddeder — geçersiz geçiş bir çakışmadır, bir doğrulama hatası değil.

## Soft delete

`tenants`, `users`, `roles`, `api_clients`'a uygulanır (`deleted_at`), çünkü
audit izlerinin kimin ne yaptığını hâlâ çözmesi gerekir. Unique index'ler
partial'dır (`WHERE deleted_at IS NULL`), böylece silinmiş bir kullanıcının
e-postası yeniden kullanılabilir.

Güvenlik-hassas durum bir soft delete'in arkasında sarkıtta bırakılmaz: bir
kullanıcıyı silmek her session'ı iptal eder ve durumu `disabled`'a taşır; bir API
client'ı iptal etmek, satır hâlâ var olsa da hiçbir token verilemeyecek şekilde
`revoked` yapar.

`audit_logs` ve `outbox_events` uygulama tarafından hiç silinmez.

## Field sınıflandırması (users)

| Sınıf | Alanlar | Görünürlük |
|---|---|---|
| Temel | `id`, `tenant_id`, `user_type`, `must_change_password`, timestamp'ler, `version` | Action izniyle |
| Korumalı | `email`, `first_name`, `last_name`, `phone`, `status`, `internal_notes`, `security_flags` | `users.read.<field>` gerekir |
| Secret | `password_hash`, `refresh_token_hash` | İzin yok — veritabanından hiç çıkmaz |

## Invariant'lar

* Platform kullanıcısının kiracısı yoktur; tenant kullanıcısının bir kiracısı olmalı (DB check constraint).
* System roller globaldir ve değiştirilemez (`chk_system_roles_global` + her yazma yolunda korumalar).
* Platform rolleri platform kullanıcılarına; tenant rolleri tenant kullanıcılarına.
* Deny, hedefe ulaşan herhangi bir scope'ta allow'u yener.
* Tutmadığın şeyi delege edemezsin (bkz. permission-delegation.md).
* Audit log'lar ekle-only — bir veritabanı trigger'ıyla zorlanır, gelenekle değil.
