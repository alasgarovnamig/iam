DELETE FROM role_permissions WHERE role_id IN (
  SELECT id FROM roles WHERE is_system
);
DELETE FROM roles WHERE is_system;
DELETE FROM permission_definitions WHERE service = 'user-management';
