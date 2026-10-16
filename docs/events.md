# Events

Every state change publishes a versioned event through the **transactional
outbox**. Business transactions never talk to Kafka directly.

## Envelope

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

The type is versioned in its name (`.v1`). A breaking change ships as `.v2`
alongside the old one until consumers migrate. `data` never carries secrets.

## Catalog

| Aggregate | Events |
|---|---|
| tenant | `tenant.created.v1`, `tenant.updated.v1` |
| user | `user.created.v1`, `user.updated.v1`, `user.status_changed.v1`, `user.role_assigned.v1`, `user.role_revoked.v1`, `user.permission_assigned.v1`, `user.permission_revoked.v1` |
| role | `role.created.v1`, `role.updated.v1`, `role.permission_assigned.v1`, `role.permission_revoked.v1` |
| api_client | `api_client.created.v1`, `api_client.secret_rotated.v1`, `api_client.revoked.v1` |

## Topics

`<prefix>.<aggregate_type>` — `wallet-gateway.tenant`, `wallet-gateway.user`,
`wallet-gateway.role`, `wallet-gateway.api-client`. The key is the aggregate id,
so events for one aggregate keep their order. Dead letters go to
`wallet-gateway.dlq` with `original_topic` and `last_error` headers.

## The outbox

```sql
INSERT INTO users …;                  -- state change
INSERT INTO outbox_events …;          -- event, SAME transaction
COMMIT;
```

If the transaction rolls back, the event goes with it — no phantom events. If
Kafka is down, rows simply wait; the business transaction is unaffected.

The worker (`cmd/worker`) polls:

```sql
SELECT … FROM outbox_events
WHERE status IN ('pending','failed') AND next_attempt_at <= now()
ORDER BY id LIMIT $batch
FOR UPDATE SKIP LOCKED;
```

`SKIP LOCKED` lets replicas share the queue without stepping on each other.
Failures back off exponentially (1s, 2s, 4s … capped at 256s); after
`OUTBOX_MAX_ATTEMPTS` the event is dead-lettered and marked `dead`.

Metrics: `wallet_gateway_outbox_worker_outbox_published_total` and
`…_outbox_failed_total`. Alert on a growing `pending` count — that is publish
lag, not load.

## Consumers must be idempotent

Delivery is at-least-once: a crash between publishing and marking `published`
re-sends the event. `event_id` is stable across retries, so consumers dedupe on
it. Ordering is guaranteed per aggregate (same key → same partition), not
globally.

## Inspecting locally

Kafka UI at http://localhost:8085, or:

```bash
docker compose exec kafka /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 --topic wallet-gateway.user --from-beginning
```
