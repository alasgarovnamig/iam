# İzin delegasyonu

**Bir principal yalnızca kendinde olan izinleri, kendi scope'una eşit veya daha
dar olarak başkasına verebilir.** Self-servis yönetimi güvenli kılan kural budur:
tenant admin'ler kendi insanlarını yönetir ama asla kendilerine verilmemiş bir
yetki üretemez.

`internal/domain/permission.go` içinde
(`PermissionSet.CanDelegate`), bir transaction içinde, yetki dağıtan her yolda
uygulanır — rol izinleri, doğrudan kullanıcı grant'ları, API client grant'ları
ve rol atama (bir rol izinlerin demetidir, atamak hepsini delege eder).

Portal, kuralı UI'ı şekillendirmek için yansıtır. Bu rehberliktir, uygulama
değil: API kontrolü tekrar yapar ve otoritedir.

## "Alt küme" ne demek

Bir grant, principal'ın tuttuğu en az bir izne göre şunların **hepsi** sağlandığında
delege edilebilir:

| Boyut | Kural |
|---|---|
| Tanım | Aynı `service.resource.action[.field]`. Bir action grant'ı alanlarını **ima etmez** ve bir field grant'ı action'ı **ima etmez**. |
| Scope | İstenen scope daha geniş değil: `own/assigned < team < organization < descendants < tenant < all`. |
| Conditions | Actor'ı bağlayan her koşul grant'ı da bağlamalı. Grant daha fazlasını ekleyebilir. |
| Deny | Tanım üzerindeki açık bir deny, onu delege etmeyi tamamen engeller. |
| Effect | Bir `deny` grant'ı, eşleşen `allow`'u tutmayı gerektirir — yetkin olmayan şeyi reddedemezsin. |

Alt-küme kontrolünün yanında iki sert sınır durur:

* Platform-only tanımlar asla bir tenant rolüne, tenant kullanıcısına veya API
  client'a ulaşamaz — actor bir şekilde onları tutuyor olsa bile.
* Tenant principal'leri asla `scope=all` alamaz.

## Örnekler

| Actor tutar | Vermeye çalışır | Sonuç |
|---|---|---|
| `users.create` @ tenant | `users.create` @ tenant | ✅ aynı |
| `users.create` @ tenant | `users.create` @ own | ✅ daha dar |
| `users.create` @ tenant | `users.delete` @ tenant | ❌ tutulmuyor |
| `users.read` @ own | `users.read` @ tenant | ❌ daha geniş scope |
| `users.read` @ tenant | `users.read` @ all | ❌ daha geniş scope |
| `users.read.email` | `users.read.phone` | ❌ farklı alan |
| `users.read` (action) | `users.read.email` | ❌ action alanı ima etmez |
| `users.read.email` | `users.read` (action) | ❌ alan action'ı ima etmez |
| `users.read` @ tenant + deny | `users.read` @ own | ❌ açıkça denied |
| `users.read` {dept: support} | `users.read` (koşulsuz) | ❌ koşullar düşürüldü |
| `users.read` {dept: support} | `users.read` {dept: support, region: az} | ✅ daha sıkı |
| Support Agent rolü | Super Admin izinleri | ❌ tutulmuyor |

## Algoritma

```
1.  Actor'ın effective izinlerini hesapla (roller ∪ doğrudan grant'lar), cache'li.
2.  İstenen grant'ı kanonik forma çevir:
        service.resource.action[.field] + scope + effect + conditions
3.  Actor hedefin kiracısı için `permissions.assign` (veya roles.assign /
    api-clients.assign) tutuyor mu — delege etme yetkisi.
4.  İlişkiyi kontrol et: actor bu hedefi yönetebilir mi? (OpenFGA; tenant
    principal'leri için kiracı sınırı mutlaktır.)
5.  Tenant subject'leri için platform-only tanımları ve scope=all'ı reddet.
6.  Yukarıdaki alt-küme kuralını uygula (CanDelegate / CanDelegateAll).
7.  Transaction içinde kalıcı yap; audit kaydı ve outbox event'iyle birlikte.
8.  OpenFGA ilişkilerini güncelle (rol atama).
9.  Cache'i geçersiz kıl: rol değişiklikleri için tenant izin versiyonunu bump'la,
    doğrudan grant'lar için principal anahtarını sil.
10. Redde: 403 PERMISSION_DELEGATION_DENIED ve girişimi ayrı bir transaction'da
    audit'le — iş transaction'ı rollback oldu ve bir yükseltme girişiminin
    kaydını kaybetmek kabul edilemez (bkz. ADR-005).
```

Toplu grant'lar (rol atama) ya-hep-ya-hiç: `CanDelegateAll` tek bir izin
delege edilebilir değilse tüm demeti reddeder, böylece kimse yarı-uygulanmış
bir rolle kalmaz.

## Neden audit ayrı bir transaction

Bir reddi hata olarak raporlanır ve bir hata transaction'ı rollback yapar —
içinde yazılmış her audit satırı dahil. Yetki-yükseltme girişimleri tam olarak
güvenlik incelemesinin istediği şeydir; bu yüzden reddler, rollback'ten sonra
kendi transaction'ını açan `auditDelegationFailure` üzerinden kaydedilir.

## Nerede test ediliyor

| Test | Kapsar |
|---|---|
| `internal/domain/permission_test.go` | 18 table-driven senaryo: aynı/dar/geniş scope, field vs action, deny önceliği, conditions, toplu red, deny delegasyonu |
| `tests/integration/authz_integration_test.go` | Kuralı gerçek HTTP üzerinden: tutulan izin ✅, platform-only ❌, geniş scope ❌ ve red audit'i |
| `web/src/auth/permissions.test.ts` | UI aynası backend ile uyuşuyor |
| `web/e2e/admin-flows.spec.ts` | İzin ağacı üzerinden grant |

## UI

`PermissionTree`, service → resource → action → field render eder. Actor'ın
delege edemeyeceği kayıtlar bir kilit ikonu ve nedenle disabled görünür ("Bu izin
için kendi scope'un 'own'; daha geniş scope veremezsin"). Scope seçici yalnızca
actor'ın gerçekten delege edebileceği scope'ları sunar; daha geniş olanlar
görünür ama disabled ve etiketlidir — böylece sınır gizemli değil okunabilir olur.
