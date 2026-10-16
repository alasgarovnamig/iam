CREATE TABLE tenants (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    slug        CITEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending', 'active', 'suspended', 'terminated')),
    settings    JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by  UUID,
    updated_by  UUID,
    version     BIGINT NOT NULL DEFAULT 1,
    deleted_at  TIMESTAMPTZ
);

CREATE UNIQUE INDEX ux_tenants_slug ON tenants (slug) WHERE deleted_at IS NULL;
CREATE INDEX ix_tenants_status ON tenants (status) WHERE deleted_at IS NULL;

CREATE TRIGGER trg_tenants_updated_at BEFORE UPDATE ON tenants
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE organization_units (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    parent_id   UUID REFERENCES organization_units (id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by  UUID,
    updated_by  UUID,
    version     BIGINT NOT NULL DEFAULT 1,
    UNIQUE (tenant_id, parent_id, name)
);

CREATE INDEX ix_org_units_tenant ON organization_units (tenant_id);

CREATE TRIGGER trg_org_units_updated_at BEFORE UPDATE ON organization_units
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
