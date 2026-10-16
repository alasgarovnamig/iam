# Event'ler

Her state değişimi, **transactional outbox** üzerinden versiyonlu bir event
yayınlar. İş transaction'ları Kafka ile asla doğrudan konuşmaz.

## Zarf (envelope)

```json
{
  "event_id": "a0e24648-ac86-421c-b6dc-e038c250f828",
  "event_type": "tenant.created.v1",
  "event_version": 1,
  "aggregate_type": "tenant",
  "aggregate_id": "a28caf1c-fe55-417f-b8e3-6bb3ef9af87a",
  "tenant_id": "a28caf1c-fe55-417f-b8e3-6bb3ef9af87a",
  "occurred_at": "2026-07-16T04:44:46.774559Z",
  "correlation_id": "7952325a-7dd7-4c66-9ca0-6138d5df1ceb",
  "causation_id": "7952325a-7dd7-4c66-9ca0-6138d5df1ceb",
  "actor": { "id": "f2fbe75f-…", "type": "platform_user" },
  "data": { "name": "Acme Payments", "slug": "acme", "status": "active" }
}
```

Tip, adında versiyonludur (`.v1`). Kırıcı bir değişiklik, consumer'lar geçene
kadar eskisinin yanında `.v2` olarak gönderilir. `data` asla secret taşımaz.

## Katalog

| Aggregate | Event'ler |
|---|---|
| tenant | `tenant.created.v1`, `tenant.updated.v1` |
| user | `user.created.v1`, `user.updated.v1`, `user.status_changed.v1`, `user.role_assigned.v1`, `user.role_revoked.v1`, `user.permission_assigned.v1`, `user.permission_revoked.v1` |
| role | `role.created.v1`, `role.updated.v1`, `role.permission_assigned.v1`, `role.permission_revoked.v1` |
| api_client | `api_client.created.v1`, `api_client.secret_rotated.v1`, `api_client.revoked.v1` |

## Topic'ler

`<prefix>.<aggregate_type>` — `wallet-gateway.tenant`, `wallet-gateway.user`,
`wallet-gateway.role`, `wallet-gateway.api-client`. Anahtar aggregate id'sidir,
böylece bir aggregate'in event'leri sıralarını korur. Dead-letter'lar
`wallet-gateway.dlq`'ya `original_topic` ve `last_error` header'larıyla gider.

## Outbox

```sql
INSERT INTO users …;                  -- state değişimi
INSERT INTO outbox_events …;          -- event, AYNI transaction
COMMIT;
```

Transaction rollback olursa event de onunla gider — hayalet event yok. Kafka
kapalıysa satırlar bekler; iş transaction'ı etkilenmez.

Worker (`cmd/worker`) poll eder:

```sql
SELECT … FROM outbox_events
WHERE status IN ('pending','failed') AND next_attempt_at <= $1
ORDER BY id LIMIT $batch
FOR UPDATE SKIP LOCKED;
```

`SKIP LOCKED`, replica'ların birbirine basmadan kuyruğu paylaşmasını sağlar.
Başarısızlıklar exponential backoff yapar (1s, 2s, 4s … 256s'de sınırlı);
`OUTBOX_MAX_ATTEMPTS`'ten sonra event dead-letter'lanır ve `dead` işaretlenir.

Metrikler: `wallet_gateway_outbox_worker_outbox_published_total` ve
`…_outbox_failed_total`. Artan bir `pending` sayısına alarm kur — bu, yük değil,
yayın gecikmesidir.

## Consumer'lar idempotent olmalı

Teslimat en-az-bir-kez: yayınlama ile `published` işaretlemesi arasında bir
çökme, event'i yeniden gönderir. `event_id` retry'ler arası kararlıdır, o yüzden
consumer'lar onunla dedupe eder. Sıralama aggregate başına garantidir (aynı
anahtar → aynı partition), global değil.

## Yerelde inceleme

Kafka UI http://localhost:8085 ya da:

```bash
docker compose exec kafka /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 --topic wallet-gateway.user --from-beginning
```
