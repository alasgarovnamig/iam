CREATE TABLE audit_logs (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    occurred_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_id       UUID,
    actor_type     TEXT NOT NULL DEFAULT 'system'
                   CHECK (actor_type IN ('platform_user','tenant_user','api_client','service_account','system')),
    tenant_id      UUID,
    action         TEXT NOT NULL,
    resource       TEXT NOT NULL,
    resource_id    TEXT,
    before_state   JSONB,
    after_state    JSONB,
    changed_fields TEXT[] NOT NULL DEFAULT '{}',
    ip             TEXT,
    user_agent     TEXT,
    request_id     TEXT,
    correlation_id TEXT,
    result         TEXT NOT NULL DEFAULT 'success' CHECK (result IN ('success','failure','denied')),
    denial_reason  TEXT
);

CREATE INDEX ix_audit_logs_tenant_time ON audit_logs (tenant_id, occurred_at DESC, id);
CREATE INDEX ix_audit_logs_actor ON audit_logs (actor_id, occurred_at DESC);
CREATE INDEX ix_audit_logs_resource ON audit_logs (resource, resource_id);

-- Audit logs are immutable: block UPDATE and DELETE at the database level.
CREATE OR REPLACE FUNCTION audit_logs_immutable() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'audit_logs is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_audit_logs_immutable
    BEFORE UPDATE OR DELETE ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION audit_logs_immutable();

CREATE TABLE outbox_events (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id        UUID NOT NULL UNIQUE,
    event_type      TEXT NOT NULL,
    event_version   INT NOT NULL DEFAULT 1,
    aggregate_type  TEXT NOT NULL,
    aggregate_id    TEXT NOT NULL,
    tenant_id       UUID,
    payload         JSONB NOT NULL,
    headers         JSONB NOT NULL DEFAULT '{}',
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','published','failed','dead')),
    attempts        INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at    TIMESTAMPTZ
);

CREATE INDEX ix_outbox_pending ON outbox_events (next_attempt_at, id)
    WHERE status IN ('pending', 'failed');
