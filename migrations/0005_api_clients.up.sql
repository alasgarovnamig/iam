CREATE TABLE api_clients (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    client_id         TEXT NOT NULL,
    secret_hash       TEXT NOT NULL,
    allowed_ips       TEXT[] NOT NULL DEFAULT '{}',
    scopes            TEXT[] NOT NULL DEFAULT '{}',
    status            TEXT NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active', 'suspended', 'expired', 'revoked')),
    expires_at        TIMESTAMPTZ,
    last_used_at      TIMESTAMPTZ,
    secret_rotated_at TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by        UUID,
    updated_by        UUID,
    version           BIGINT NOT NULL DEFAULT 1,
    deleted_at        TIMESTAMPTZ
);

CREATE UNIQUE INDEX ux_api_clients_client_id ON api_clients (client_id);
CREATE INDEX ix_api_clients_tenant ON api_clients (tenant_id, status) WHERE deleted_at IS NULL;

CREATE TRIGGER trg_api_clients_updated_at BEFORE UPDATE ON api_clients
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE api_client_permissions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    api_client_id UUID NOT NULL REFERENCES api_clients (id) ON DELETE CASCADE,
    tenant_id     UUID NOT NULL,
    permission_id UUID NOT NULL REFERENCES permission_definitions (id) ON DELETE CASCADE,
    scope         TEXT NOT NULL DEFAULT 'tenant'
                  CHECK (scope IN ('own','assigned','team','organization','tenant','descendants','all')),
    effect        TEXT NOT NULL DEFAULT 'allow' CHECK (effect IN ('allow', 'deny')),
    conditions    JSONB NOT NULL DEFAULT '{}',
    granted_by    UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (api_client_id, permission_id, scope, effect)
);

CREATE INDEX ix_api_client_permissions_client ON api_client_permissions (api_client_id);
