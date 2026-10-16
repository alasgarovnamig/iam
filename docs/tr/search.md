# Dinamik arama

Bir kaynak, sabit bir dizi sorgu parametresi yerine **dinamik bir koşul ağacıyla**
aranabilir — alanlar AND/OR ve karşılaştırma operatörleriyle birleştirilir. Aynı
motor `users`, `tenants` ve `api-clients`'ı destekler.

Her aramayı iki yetki aşaması korur:

1. **Action seviyesi** — actor bu kaynakta aramayı hiç çağırabilir mi
   (`<resource>.search`)?
2. **Field seviyesi** — filtrede veya sort'ta geçen **her** alan için actor
   `<resource>.search.<field>` tutuyor mu? Eksik grant, adıyla reddedilir
   (`403 FIELD_NOT_ALLOWED`, ör. `search:status`).

Bir alanı okumak ve onunla aramak farklı yeteneklerdir: bir destek elemanı bir
müşterinin durumunu görebilir ama tüm kiracıyı duruma göre sayamaz.

## İstek

```http
POST /api/v1/users/search
POST /api/v1/tenants/search
POST /api/v1/api-clients/search
```

```jsonc
{
  "filter": {
    "op": "and",                       // grup: "and" | "or"
    "conditions": [
      { "field": "first_name", "op": "eq",   "value": "Elam" },     // yaprak
      { "op": "or", "conditions": [
          { "field": "last_name", "op": "ilike", "value": "swn%" },
          { "field": "status",    "op": "in",    "value": ["active","pending"] }
      ]}
    ]
  },
  "sort": [{ "field": "created_at", "dir": "desc" }],
  "page": 1,
  "page_size": 20
}
```

`field` içeren bir düğüm **yapraktır** (`op`'u bir karşılaştırma); aksi halde
**gruptur** (`op`'u `conditions` ile `and`/`or`). Gruplar `MaxFilterDepth` (6)
seviyeye kadar iç içe geçebilir. Cevap standart liste zarfıdır ve sonuç satırları
yine **read-field filtresinden** geçer — okuyamayacağın bir alanla filtre
yapabilirsin ama okuma iznin olmayan bir alanı asla görmezsin.

## Alan tipine göre operatörler

| Alan tipi | Operatörler |
|---|---|
| metin (`email`, `name`, `slug`, …) | `eq`, `neq`, `like`, `ilike`, `in` |
| enum (`status`) | `eq`, `neq`, `in` |
| timestamp (`created_at`) | `eq`, `gt`, `gte`, `lt`, `lte` |

`created_at` bir temel alandır: onunla filtrelemek/sıralamak yalnızca action-level
`search` iznini gerektirir, alan-bazlı grant değil.

## Kaynak başına aranabilir alanlar

| Kaynak | Alanlar |
|---|---|
| users | email, first_name, last_name, phone, status, created_at |
| tenants | name, slug, status, created_at |
| api-clients | name, client_id, status, created_at |

Alan kümesi `domain/search.go`'da tek doğruluk kaynağıdır (`UsersSearchSchema`,
`TenantsSearchSchema`, `APIClientsSearchSchema`). Hem izin kontrolünü hem SQL
üreticisini kısıtlar.

## Nasıl güvenli kalıyor

Sorgu şekli tamamen dinamik olsa da SQL-injection yüzeyi yoktur:

* **Kolon adları** yalnızca kaynak şemasının allow-list'inden gelir — asla istek
  gövdesinden.
* **Operatörler** sabit bir tablo üzerinden eşlenir.
* **Her değer** bir bind parametresidir (`$N`); `in`, tek bir dizi parametresiyle
  `col = ANY($n)` kullanır.
* İç içe geçme derinliği sınırlıdır; bilinmeyen alanlar / yasadışı operatörler /
  bozuk değer şekilleri veritabanına ulaşmadan `400` ile reddedilir.

`x'; DROP TABLE users; --` gibi bir değer veri olarak ele alınır: bir bind
parametresi olur, hiçbir şeyle eşleşmez ve tablo dokunulmadan kalır
(`internal/postgres/search_test.go` ile test edilir).

## Nerede yaşıyor

| Katman | Dosya | Sorumluluk |
|---|---|---|
| Domain | `internal/domain/search.go` | Ağaç modeli, şemalar, doğrulama, alan toplama |
| Adapter | `internal/postgres/search.go` | Parametreli WHERE + güvenli ORDER BY üreticisi |
| Application | `internal/app/search.go` | İki-aşamalı yetki; servis-başına `Search` use-case'i |
| HTTP | `internal/httpapi/handlers_search.go` | İstek DTO → komut; üç endpoint |
| Frontend | `web/src/components/SearchBuilder.tsx` | İzin-farkında sorgu kurucu |

## Testler

| Test | Kapsar |
|---|---|
| `domain/search_test.go` | Doğrulama: bilinmeyen alan, yasadışı operatör, enum değerleri, derinlik limiti, `Fields()` |
| `internal/postgres/search_test.go` | Parametrelendirme, injection payload'ı veri kalır, timestamp cast, ORDER BY allow-list |
| `tests/integration` scripti | Canlı: AND/OR/LIKE/IN, alan-bazlı adıyla 403, injection zararsız |
| `admin-portal/src/api/search.test.ts` | UI değer dönüşümleri (`contains` → `%v%`), istek montajı |

## UI

`SearchBuilder`, tek bir AND/OR anahtarıyla birleştirilmiş düz bir koşul listesi
render eder — "(name AND slug)", "(name OR slug contains …)" için yeterli.
Yalnızca actor'ın arayabileceği alanları sunar (`searchableFields()`,
`search.<field>`'a göre filtreler), böylece aranamaz bir alan seçilemez bile.
Dostça operatörler wire operatörlerine eşlenir (`contains` → `ilike %v%`,
`starts with` → `ilike v%`). Backend her şeyi yine de yeniden uygular — UI
rehberliktir, sınır değil.
