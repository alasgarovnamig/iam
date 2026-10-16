-- Row-Level Security.
--
-- The application connects as `wallet_app` (not the table owner), so RLS is
-- enforced for every application query. The transaction manager sets:
--   SET LOCAL app.tenant_id    = '<uuid>'   (tenant principals)
--   SET LOCAL app.is_platform  = 'true'     (platform principals / workers)
-- Both GUCs default to unset => deny by default (no rows visible).

CREATE OR REPLACE FUNCTION app_is_platform() RETURNS boolean AS $$
  SELECT COALESCE(current_setting('app.is_platform', true), '') = 'true';
$$ LANGUAGE sql STABLE;

CREATE OR REPLACE FUNCTION app_tenant_id() RETURNS uuid AS $$
  SELECT NULLIF(current_setting('app.tenant_id', true), '')::uuid;
$$ LANGUAGE sql STABLE;

-- ── tenants ─────────────────────────────────────────────────────────────────
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenants_platform_all ON tenants
    USING (app_is_platform()) WITH CHECK (app_is_platform());
CREATE POLICY tenants_self_read ON tenants FOR SELECT
    USING (id = app_tenant_id());
CREATE POLICY tenants_self_update ON tenants FOR UPDATE
    USING (id = app_tenant_id()) WITH CHECK (id = app_tenant_id());

-- ── generic tenant-scoped tables ───────────────────────────────────────────
-- users
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
CREATE POLICY users_platform_all ON users
    USING (app_is_platform()) WITH CHECK (app_is_platform());
CREATE POLICY users_tenant ON users
    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- user_credentials
ALTER TABLE user_credentials ENABLE ROW LEVEL SECURITY;
CREATE POLICY user_credentials_platform_all ON user_credentials
    USING (app_is_platform()) WITH CHECK (app_is_platform());
CREATE POLICY user_credentials_tenant ON user_credentials
    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- sessions
ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
CREATE POLICY sessions_platform_all ON sessions
    USING (app_is_platform()) WITH CHECK (app_is_platform());
CREATE POLICY sessions_tenant ON sessions
    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- organization_units
ALTER TABLE organization_units ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_units_platform_all ON organization_units
    USING (app_is_platform()) WITH CHECK (app_is_platform());
CREATE POLICY org_units_tenant ON organization_units
    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- roles: tenants also read global system roles (tenant_id IS NULL, is_system)
ALTER TABLE roles ENABLE ROW LEVEL SECURITY;
CREATE POLICY roles_platform_all ON roles
    USING (app_is_platform()) WITH CHECK (app_is_platform());
CREATE POLICY roles_tenant_own ON roles
    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY roles_system_read ON roles FOR SELECT
    USING (tenant_id IS NULL AND is_system);

-- role_permissions: system role rows (tenant_id NULL) readable by tenants
ALTER TABLE role_permissions ENABLE ROW LEVEL SECURITY;
CREATE POLICY role_permissions_platform_all ON role_permissions
    USING (app_is_platform()) WITH CHECK (app_is_platform());
CREATE POLICY role_permissions_tenant_own ON role_permissions
    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY role_permissions_system_read ON role_permissions FOR SELECT
    USING (tenant_id IS NULL);

-- user_roles
ALTER TABLE user_roles ENABLE ROW LEVEL SECURITY;
CREATE POLICY user_roles_platform_all ON user_roles
    USING (app_is_platform()) WITH CHECK (app_is_platform());
CREATE POLICY user_roles_tenant ON user_roles
    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- user_permissions
ALTER TABLE user_permissions ENABLE ROW LEVEL SECURITY;
CREATE POLICY user_permissions_platform_all ON user_permissions
    USING (app_is_platform()) WITH CHECK (app_is_platform());
CREATE POLICY user_permissions_tenant ON user_permissions
    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- api_clients
ALTER TABLE api_clients ENABLE ROW LEVEL SECURITY;
CREATE POLICY api_clients_platform_all ON api_clients
    USING (app_is_platform()) WITH CHECK (app_is_platform());
CREATE POLICY api_clients_tenant ON api_clients
    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- api_client_permissions
ALTER TABLE api_client_permissions ENABLE ROW LEVEL SECURITY;
CREATE POLICY api_client_permissions_platform_all ON api_client_permissions
    USING (app_is_platform()) WITH CHECK (app_is_platform());
CREATE POLICY api_client_permissions_tenant ON api_client_permissions
    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- audit_logs: append-only; tenants read their own, platform reads all
ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
CREATE POLICY audit_logs_platform_all ON audit_logs
    USING (app_is_platform()) WITH CHECK (app_is_platform());
CREATE POLICY audit_logs_tenant_read ON audit_logs FOR SELECT
    USING (tenant_id = app_tenant_id());
CREATE POLICY audit_logs_tenant_insert ON audit_logs FOR INSERT
    WITH CHECK (tenant_id = app_tenant_id());

-- outbox_events: tenant transactions append their own events; worker is platform
ALTER TABLE outbox_events ENABLE ROW LEVEL SECURITY;
CREATE POLICY outbox_platform_all ON outbox_events
    USING (app_is_platform()) WITH CHECK (app_is_platform());
CREATE POLICY outbox_tenant_insert ON outbox_events FOR INSERT
    WITH CHECK (tenant_id = app_tenant_id());

-- permission_definitions: global read-only catalog (no tenant data) — RLS not
-- needed; write access is restricted by grants (owner only).
REVOKE INSERT, UPDATE, DELETE ON permission_definitions FROM wallet_app;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO wallet_app;
GRANT INSERT, UPDATE, DELETE ON tenants, organization_units, users, user_credentials,
    sessions, roles, role_permissions, user_roles, user_permissions,
    api_clients, api_client_permissions, outbox_events TO wallet_app;
GRANT INSERT ON audit_logs TO wallet_app;
REVOKE UPDATE, DELETE ON audit_logs FROM wallet_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO wallet_app;
