-- Removing the definitions cascades to their role_permissions rows
-- (role_permissions.permission_id has ON DELETE CASCADE).
DELETE FROM permission_definitions
WHERE service = 'user-management'
  AND (
    (resource = 'tenants' AND action = 'search' AND field IS NOT NULL)
    OR (resource = 'api-clients' AND action = 'search')
  );
