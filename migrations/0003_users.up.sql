CREATE TABLE users (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- NULL tenant_id = platform user
    tenant_id            UUID REFERENCES tenants (id) ON DELETE RESTRICT,
    organization_unit_id UUID REFERENCES organization_units (id) ON DELETE SET NULL,
    user_type            TEXT NOT NULL DEFAULT 'tenant_user'
                         CHECK (user_type IN ('platform_user', 'tenant_user', 'service_account')),
    email                CITEXT NOT NULL,
    first_name           TEXT NOT NULL DEFAULT '',
    last_name            TEXT NOT NULL DEFAULT '',
    phone                TEXT,
    status               TEXT NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending', 'active', 'suspended', 'locked', 'disabled')),
    must_change_password BOOLEAN NOT NULL DEFAULT false,
    internal_notes       TEXT,
    security_flags       JSONB NOT NULL DEFAULT '{}',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by           UUID,
    updated_by           UUID,
    version              BIGINT NOT NULL DEFAULT 1,
    deleted_at           TIMESTAMPTZ,
    CONSTRAINT chk_platform_user_tenant CHECK (
        (user_type = 'platform_user' AND tenant_id IS NULL) OR
        (user_type <> 'platform_user' AND tenant_id IS NOT NULL)
    )
);

-- Email unique per tenant; platform users (tenant_id NULL) unique globally.
CREATE UNIQUE INDEX ux_users_tenant_email
    ON users (COALESCE(tenant_id, '00000000-0000-0000-0000-000000000000'::uuid), email)
    WHERE deleted_at IS NULL;
CREATE INDEX ix_users_tenant_status ON users (tenant_id, status) WHERE deleted_at IS NULL;
CREATE INDEX ix_users_tenant_created ON users (tenant_id, created_at DESC, id) WHERE deleted_at IS NULL;

CREATE TRIGGER trg_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE user_credentials (
    user_id               UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    tenant_id             UUID,
    password_hash         TEXT NOT NULL,
    password_changed_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    failed_login_attempts INT NOT NULL DEFAULT 0,
    locked_until          TIMESTAMPTZ,
    last_login_at         TIMESTAMPTZ,
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_user_credentials_updated_at BEFORE UPDATE ON user_credentials
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE sessions (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    tenant_id          UUID,
    family_id          UUID NOT NULL,
    refresh_token_hash TEXT NOT NULL,
    status             TEXT NOT NULL DEFAULT 'active'
                       CHECK (status IN ('active', 'rotated', 'revoked', 'reused')),
    ip                 TEXT,
    user_agent         TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at         TIMESTAMPTZ NOT NULL,
    revoked_at         TIMESTAMPTZ,
    revoke_reason      TEXT
);

CREATE UNIQUE INDEX ux_sessions_refresh_hash ON sessions (refresh_token_hash);
CREATE INDEX ix_sessions_user ON sessions (user_id, status);
CREATE INDEX ix_sessions_family ON sessions (family_id);
CREATE INDEX ix_sessions_expiry ON sessions (expires_at) WHERE status = 'active';
