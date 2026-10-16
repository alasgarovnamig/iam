-- Permission catalog + default system roles. Idempotent (ON CONFLICT DO NOTHING).

-- ── Permission definitions ─────────────────────────────────────────────────
INSERT INTO permission_definitions (service, resource, action, field, description, is_platform_only) VALUES
-- tenants
('user-management','tenants','create', NULL,'Create tenants', true),
('user-management','tenants','read',   NULL,'Read tenant details', false),
('user-management','tenants','update', NULL,'Update tenant', false),
('user-management','tenants','delete', NULL,'Delete (terminate) tenant', true),
('user-management','tenants','list',   NULL,'List tenants', true),
('user-management','tenants','search', NULL,'Search tenants', true),
-- users (action level)
('user-management','users','create', NULL,'Create users', false),
('user-management','users','read',   NULL,'Read users (base fields)', false),
('user-management','users','update', NULL,'Update users (base fields)', false),
('user-management','users','delete', NULL,'Delete users', false),
('user-management','users','list',   NULL,'List users', false),
('user-management','users','search', NULL,'Search users (base fields)', false),
-- users (field level: read)
('user-management','users','read','email',         'Read user email', false),
('user-management','users','read','first_name',    'Read user first name', false),
('user-management','users','read','last_name',     'Read user last name', false),
('user-management','users','read','phone',         'Read user phone', false),
('user-management','users','read','status',        'Read user status', false),
('user-management','users','read','internal_notes','Read internal notes', false),
('user-management','users','read','security_flags','Read security flags', true),
-- users (field level: update)
('user-management','users','update','email',         'Update user email', false),
('user-management','users','update','first_name',    'Update user first name', false),
('user-management','users','update','last_name',     'Update user last name', false),
('user-management','users','update','phone',         'Update user phone', false),
('user-management','users','update','status',        'Update user status', false),
('user-management','users','update','internal_notes','Update internal notes', false),
-- users (field level: search)
('user-management','users','search','email',      'Filter users by email', false),
('user-management','users','search','first_name', 'Filter users by first name', false),
('user-management','users','search','last_name',  'Filter users by last name', false),
('user-management','users','search','phone',      'Filter users by phone', false),
('user-management','users','search','status',     'Filter users by status', false),
-- roles
('user-management','roles','create', NULL,'Create roles', false),
('user-management','roles','read',   NULL,'Read roles', false),
('user-management','roles','update', NULL,'Update roles', false),
('user-management','roles','delete', NULL,'Delete roles', false),
('user-management','roles','list',   NULL,'List roles', false),
('user-management','roles','assign', NULL,'Assign roles to users', false),
('user-management','roles','revoke', NULL,'Revoke roles from users', false),
-- permissions (catalog + direct grants)
('user-management','permissions','read',  NULL,'Read permission catalog', false),
('user-management','permissions','list',  NULL,'List permission catalog', false),
('user-management','permissions','assign',NULL,'Grant direct/role permissions', false),
('user-management','permissions','revoke',NULL,'Revoke direct/role permissions', false),
-- api clients
('user-management','api-clients','create', NULL,'Create API clients', false),
('user-management','api-clients','read',   NULL,'Read API clients', false),
('user-management','api-clients','update', NULL,'Update API clients', false),
('user-management','api-clients','delete', NULL,'Revoke API clients', false),
('user-management','api-clients','list',   NULL,'List API clients', false),
('user-management','api-clients','rotate', NULL,'Rotate API client secrets', false),
('user-management','api-clients','assign', NULL,'Grant API client permissions', false),
('user-management','api-clients','revoke', NULL,'Revoke API client permissions', false),
-- audit logs
('user-management','audit-logs','read',   NULL,'Read audit logs', false),
('user-management','audit-logs','list',   NULL,'List audit logs', false),
('user-management','audit-logs','search', NULL,'Search audit logs', false),
('user-management','audit-logs','export', NULL,'Export audit logs', false),
-- sessions
('user-management','sessions','read',   NULL,'Read sessions', false),
('user-management','sessions','list',   NULL,'List sessions', false),
('user-management','sessions','delete', NULL,'Revoke sessions', false)
ON CONFLICT (service, resource, action, COALESCE(field, '')) DO NOTHING;

-- ── System roles (fixed UUIDs) ─────────────────────────────────────────────
INSERT INTO roles (id, tenant_id, name, description, role_type, is_system, status) VALUES
('a1000000-0000-4000-8000-000000000001', NULL, 'Super Admin',        'Full platform access',                'platform', true, 'active'),
('a1000000-0000-4000-8000-000000000002', NULL, 'Platform Admin',     'Platform administration',             'platform', true, 'active'),
('a1000000-0000-4000-8000-000000000003', NULL, 'Operations Admin',   'Platform operations',                 'platform', true, 'active'),
('a1000000-0000-4000-8000-000000000004', NULL, 'Support Agent',      'Platform support (read-mostly)',      'platform', true, 'active'),
('a1000000-0000-4000-8000-000000000005', NULL, 'Compliance Officer', 'Compliance and audit review',         'platform', true, 'active'),
('a1000000-0000-4000-8000-000000000006', NULL, 'Platform Auditor',   'Read-only audit access',              'platform', true, 'active'),
('a1000000-0000-4000-8000-000000000007', NULL, 'Platform Read Only', 'Read-only platform access',           'platform', true, 'active'),
('a2000000-0000-4000-8000-000000000001', NULL, 'Tenant Owner',       'Tenant owner: full tenant access',    'tenant',   true, 'active'),
('a2000000-0000-4000-8000-000000000002', NULL, 'Tenant Admin',       'Tenant administration',               'tenant',   true, 'active'),
('a2000000-0000-4000-8000-000000000003', NULL, 'Tenant Developer',   'API client / integration management', 'tenant',   true, 'active'),
('a2000000-0000-4000-8000-000000000004', NULL, 'Tenant Operations',  'Tenant user operations',              'tenant',   true, 'active'),
('a2000000-0000-4000-8000-000000000005', NULL, 'Tenant Finance',     'Finance: read users and audit',       'tenant',   true, 'active'),
('a2000000-0000-4000-8000-000000000006', NULL, 'Tenant Support',     'Tenant support (read-mostly)',        'tenant',   true, 'active'),
('a2000000-0000-4000-8000-000000000007', NULL, 'Tenant Auditor',     'Tenant read-only audit',              'tenant',   true, 'active'),
('a2000000-0000-4000-8000-000000000008', NULL, 'Tenant Read Only',   'Tenant read-only',                    'tenant',   true, 'active'),
('a2000000-0000-4000-8000-000000000009', NULL, 'API Integration Manager', 'Manage API clients only',        'tenant',   true, 'active')
ON CONFLICT (id) DO NOTHING;

-- ── Role permissions ────────────────────────────────────────────────────────
-- Super Admin: everything, scope=all
INSERT INTO role_permissions (role_id, permission_id, scope, effect)
SELECT 'a1000000-0000-4000-8000-000000000001', id, 'all', 'allow' FROM permission_definitions
ON CONFLICT DO NOTHING;

-- Platform Admin: everything except security_flags read and audit export
INSERT INTO role_permissions (role_id, permission_id, scope, effect)
SELECT 'a1000000-0000-4000-8000-000000000002', id, 'all', 'allow' FROM permission_definitions
WHERE NOT (resource = 'users' AND action = 'read' AND field = 'security_flags')
  AND NOT (resource = 'audit-logs' AND action = 'export')
ON CONFLICT DO NOTHING;

-- Operations Admin: tenants read/list/update, users CRU + fields (no delete), sessions
INSERT INTO role_permissions (role_id, permission_id, scope, effect)
SELECT 'a1000000-0000-4000-8000-000000000003', id, 'all', 'allow' FROM permission_definitions
WHERE (resource = 'tenants' AND action IN ('read','list','update','search'))
   OR (resource = 'users' AND action IN ('create','read','update','list','search')
       AND (field IS NULL OR field NOT IN ('security_flags','internal_notes')))
   OR (resource = 'sessions')
   OR (resource = 'roles' AND action IN ('read','list'))
   OR (resource = 'permissions' AND action IN ('read','list'))
ON CONFLICT DO NOTHING;

-- Support Agent: read-mostly, limited fields, NO email search
INSERT INTO role_permissions (role_id, permission_id, scope, effect)
SELECT 'a1000000-0000-4000-8000-000000000004', id, 'all', 'allow' FROM permission_definitions
WHERE (resource = 'users' AND action = 'read' AND (field IS NULL OR field IN ('email','first_name','last_name','status')))
   OR (resource = 'users' AND action = 'list')
   OR (resource = 'users' AND action = 'search' AND (field IS NULL OR field IN ('first_name','last_name','status')))
   OR (resource = 'sessions' AND action IN ('read','list'))
   OR (resource = 'tenants' AND action IN ('read','list'))
ON CONFLICT DO NOTHING;

-- Compliance Officer
INSERT INTO role_permissions (role_id, permission_id, scope, effect)
SELECT 'a1000000-0000-4000-8000-000000000005', id, 'all', 'allow' FROM permission_definitions
WHERE (resource = 'audit-logs')
   OR (resource = 'users' AND action IN ('read','list','search') AND (field IS NULL OR field <> 'security_flags'))
   OR (resource = 'tenants' AND action IN ('read','list','search'))
ON CONFLICT DO NOTHING;

-- Platform Auditor: read/list/search everywhere + audit export
INSERT INTO role_permissions (role_id, permission_id, scope, effect)
SELECT 'a1000000-0000-4000-8000-000000000006', id, 'all', 'allow' FROM permission_definitions
WHERE action IN ('read','list','search','export')
ON CONFLICT DO NOTHING;

-- Platform Read Only: read/list, without sensitive user fields
INSERT INTO role_permissions (role_id, permission_id, scope, effect)
SELECT 'a1000000-0000-4000-8000-000000000007', id, 'all', 'allow' FROM permission_definitions
WHERE action IN ('read','list')
  AND (field IS NULL OR field NOT IN ('security_flags','internal_notes'))
ON CONFLICT DO NOTHING;

-- Tenant Owner: all tenant-grantable permissions, scope=tenant
INSERT INTO role_permissions (role_id, permission_id, scope, effect)
SELECT 'a2000000-0000-4000-8000-000000000001', id, 'tenant', 'allow' FROM permission_definitions
WHERE NOT is_platform_only
ON CONFLICT DO NOTHING;

-- Tenant Admin: like owner, minus tenant update and internal notes
INSERT INTO role_permissions (role_id, permission_id, scope, effect)
SELECT 'a2000000-0000-4000-8000-000000000002', id, 'tenant', 'allow' FROM permission_definitions
WHERE NOT is_platform_only
  AND NOT (resource = 'tenants' AND action = 'update')
  AND (field IS NULL OR field <> 'internal_notes')
ON CONFLICT DO NOTHING;

-- Tenant Developer
INSERT INTO role_permissions (role_id, permission_id, scope, effect)
SELECT 'a2000000-0000-4000-8000-000000000003', id, 'tenant', 'allow' FROM permission_definitions
WHERE (resource = 'api-clients')
   OR (resource = 'users' AND action = 'read' AND (field IS NULL OR field IN ('first_name','last_name','status')))
   OR (resource = 'users' AND action = 'list')
   OR (resource = 'permissions' AND action IN ('read','list'))
ON CONFLICT DO NOTHING;

-- Tenant Operations: user CRU with base fields, no email search
INSERT INTO role_permissions (role_id, permission_id, scope, effect)
SELECT 'a2000000-0000-4000-8000-000000000004', id, 'tenant', 'allow' FROM permission_definitions
WHERE (resource = 'users' AND action IN ('read','update','list')
       AND (field IS NULL OR field IN ('email','first_name','last_name','phone','status')))
   OR (resource = 'users' AND action = 'search' AND (field IS NULL OR field IN ('first_name','last_name','status')))
   OR (resource = 'sessions' AND action IN ('read','list'))
ON CONFLICT DO NOTHING;

-- Tenant Finance
INSERT INTO role_permissions (role_id, permission_id, scope, effect)
SELECT 'a2000000-0000-4000-8000-000000000005', id, 'tenant', 'allow' FROM permission_definitions
WHERE (resource = 'audit-logs' AND action IN ('read','list','search'))
   OR (resource = 'users' AND action IN ('read','list') AND (field IS NULL OR field IN ('first_name','last_name','status')))
ON CONFLICT DO NOTHING;

-- Tenant Support: read users (basic fields) and sessions
INSERT INTO role_permissions (role_id, permission_id, scope, effect)
SELECT 'a2000000-0000-4000-8000-000000000006', id, 'tenant', 'allow' FROM permission_definitions
WHERE (resource = 'users' AND action IN ('read','list') AND (field IS NULL OR field IN ('first_name','last_name','status')))
   OR (resource = 'sessions' AND action IN ('read','list'))
ON CONFLICT DO NOTHING;

-- Tenant Auditor
INSERT INTO role_permissions (role_id, permission_id, scope, effect)
SELECT 'a2000000-0000-4000-8000-000000000007', id, 'tenant', 'allow' FROM permission_definitions
WHERE NOT is_platform_only AND action IN ('read','list','search')
ON CONFLICT DO NOTHING;

-- Tenant Read Only
INSERT INTO role_permissions (role_id, permission_id, scope, effect)
SELECT 'a2000000-0000-4000-8000-000000000008', id, 'tenant', 'allow' FROM permission_definitions
WHERE NOT is_platform_only AND action IN ('read','list')
  AND (field IS NULL OR field NOT IN ('internal_notes','security_flags'))
ON CONFLICT DO NOTHING;

-- API Integration Manager
INSERT INTO role_permissions (role_id, permission_id, scope, effect)
SELECT 'a2000000-0000-4000-8000-000000000009', id, 'tenant', 'allow' FROM permission_definitions
WHERE resource = 'api-clients' OR (resource = 'permissions' AND action IN ('read','list'))
ON CONFLICT DO NOTHING;
