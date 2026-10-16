-- Field-level search permissions for tenants and api-clients, so the dynamic
-- search engine can enforce search.<field> on those resources too (users
-- already had them from 0008). Idempotent.

-- ── New permission definitions ──────────────────────────────────────────────
INSERT INTO permission_definitions (service, resource, action, field, description, is_platform_only) VALUES
-- tenants search is platform-only (mirrors the existing tenants.search action)
('user-management','tenants','search','name',  'Filter tenants by name',   true),
('user-management','tenants','search','slug',  'Filter tenants by slug',   true),
('user-management','tenants','search','status','Filter tenants by status', true),
-- api-clients gains a search action + searchable fields (tenant-grantable)
('user-management','api-clients','search', NULL,       'Search API clients',            false),
('user-management','api-clients','search','name',      'Filter API clients by name',    false),
('user-management','api-clients','search','client_id', 'Filter API clients by client id', false),
('user-management','api-clients','search','status',    'Filter API clients by status',  false)
ON CONFLICT (service, resource, action, COALESCE(field, '')) DO NOTHING;

-- ── Grant the new tenants.search.<field> to every role holding tenants.search ─
INSERT INTO role_permissions (role_id, tenant_id, permission_id, scope, effect)
SELECT rp.role_id, rp.tenant_id, pd.id, rp.scope, rp.effect
FROM role_permissions rp
JOIN permission_definitions base ON base.id = rp.permission_id
    AND base.resource = 'tenants' AND base.action = 'search' AND base.field IS NULL
JOIN permission_definitions pd
    ON pd.resource = 'tenants' AND pd.action = 'search' AND pd.field IS NOT NULL
ON CONFLICT (role_id, permission_id, scope, effect) DO NOTHING;

-- ── Grant api-clients search (action + fields) to every role that can list ───
-- them. "You can enumerate the clients" ⇒ "you can search them", at the same
-- scope the list grant already uses.
INSERT INTO role_permissions (role_id, tenant_id, permission_id, scope, effect)
SELECT rp.role_id, rp.tenant_id, pd.id, rp.scope, rp.effect
FROM role_permissions rp
JOIN permission_definitions base ON base.id = rp.permission_id
    AND base.resource = 'api-clients' AND base.action = 'list' AND base.field IS NULL
JOIN permission_definitions pd
    ON pd.resource = 'api-clients' AND pd.action = 'search'
ON CONFLICT (role_id, permission_id, scope, effect) DO NOTHING;
