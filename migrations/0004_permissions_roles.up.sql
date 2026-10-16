CREATE TABLE permission_definitions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service          TEXT NOT NULL,
    resource         TEXT NOT NULL,
    action           TEXT NOT NULL
                     CHECK (action IN ('create','read','update','delete','search','list',
                                       'assign','revoke','export','rotate')),
    -- NULL field = the action itself (all standard fields); non-null = field-level grant
    field            TEXT,
    description      TEXT NOT NULL DEFAULT '',
    is_platform_only BOOLEAN NOT NULL DEFAULT false,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ux_permission_defs
    ON permission_definitions (service, resource, action, COALESCE(field, ''));

CREATE TABLE roles (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- NULL tenant_id = system role (platform-defined, immutable by tenants)
    tenant_id      UUID REFERENCES tenants (id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    role_type      TEXT NOT NULL CHECK (role_type IN ('platform', 'tenant')),
    is_system      BOOLEAN NOT NULL DEFAULT false,
    status         TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    parent_role_id UUID REFERENCES roles (id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by     UUID,
    updated_by     UUID,
    version        BIGINT NOT NULL DEFAULT 1,
    deleted_at     TIMESTAMPTZ,
    CONSTRAINT chk_system_roles_global CHECK (NOT is_system OR tenant_id IS NULL)
);

CREATE UNIQUE INDEX ux_roles_tenant_name
    ON roles (COALESCE(tenant_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(name))
    WHERE deleted_at IS NULL;
CREATE INDEX ix_roles_tenant ON roles (tenant_id) WHERE deleted_at IS NULL;

CREATE TRIGGER trg_roles_updated_at BEFORE UPDATE ON roles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE role_permissions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id       UUID NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    tenant_id     UUID,
    permission_id UUID NOT NULL REFERENCES permission_definitions (id) ON DELETE CASCADE,
    scope         TEXT NOT NULL DEFAULT 'tenant'
                  CHECK (scope IN ('own','assigned','team','organization','tenant','descendants','all')),
    effect        TEXT NOT NULL DEFAULT 'allow' CHECK (effect IN ('allow', 'deny')),
    conditions    JSONB NOT NULL DEFAULT '{}',
    created_by    UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (role_id, permission_id, scope, effect)
);

CREATE INDEX ix_role_permissions_role ON role_permissions (role_id);

CREATE TABLE user_roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id     UUID NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    tenant_id   UUID,
    assigned_by UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, role_id)
);

CREATE INDEX ix_user_roles_user ON user_roles (user_id);
CREATE INDEX ix_user_roles_role ON user_roles (role_id);

CREATE TABLE user_permissions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    tenant_id     UUID,
    permission_id UUID NOT NULL REFERENCES permission_definitions (id) ON DELETE CASCADE,
    scope         TEXT NOT NULL DEFAULT 'own'
                  CHECK (scope IN ('own','assigned','team','organization','tenant','descendants','all')),
    effect        TEXT NOT NULL DEFAULT 'allow' CHECK (effect IN ('allow', 'deny')),
    conditions    JSONB NOT NULL DEFAULT '{}',
    granted_by    UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ,
    UNIQUE (user_id, permission_id, scope, effect)
);

CREATE INDEX ix_user_permissions_user ON user_permissions (user_id);
