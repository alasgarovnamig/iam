import { describe, expect, it } from 'vitest';
import { PermissionSet, parsePermission, parsePermissions } from './permissions';
import { SCOPES } from '../api/types';

/**
 * The UI mirror of the delegation rule. It must agree with the Go engine in
 * internal/domain/permission.go — when it does
 * not, the API rejects what the UI offered, which is a UX bug (the backend
 * still holds the line).
 */

describe('parsePermission', () => {
  it('parses a "code:scope" entry from /auth/me', () => {
    expect(parsePermission('user-management.users.read.email:tenant')).toEqual({
      code: 'user-management.users.read.email',
      scope: 'tenant',
      effect: 'allow',
    });
  });

  it('rejects malformed entries instead of guessing', () => {
    expect(parsePermission('no-scope-here')).toBeNull();
    expect(parsePermission('user-management.users.read:universe')).toBeNull();
    expect(parsePermission('')).toBeNull();
  });

  it('drops unparseable entries from a batch', () => {
    const parsed = parsePermissions(['user-management.users.read:tenant', 'garbage', 'x:nope']);
    expect(parsed).toHaveLength(1);
  });
});

describe('PermissionSet.has', () => {
  const set = PermissionSet.fromRaw([
    'user-management.users.read:tenant',
    'user-management.users.read.email:tenant',
  ]);

  it('is true for a held permission', () => {
    expect(set.has('user-management.users.read')).toBe(true);
  });

  it('is false for a permission that is not held', () => {
    expect(set.has('user-management.users.delete')).toBe(false);
  });

  it('does not treat an action grant as a field grant', () => {
    expect(set.has('user-management.users.read.phone')).toBe(false);
  });

  it('hasAny is true when at least one code is held', () => {
    expect(set.hasAny('user-management.users.delete', 'user-management.users.read')).toBe(true);
    expect(set.hasAny('user-management.users.delete', 'user-management.roles.create')).toBe(false);
  });

  it('lets deny override allow', () => {
    const denied = new PermissionSet([
      { code: 'user-management.users.delete', scope: 'tenant', effect: 'allow' },
      { code: 'user-management.users.delete', scope: 'tenant', effect: 'deny' },
    ]);
    expect(denied.has('user-management.users.delete')).toBe(false);
  });
});

describe('PermissionSet.maxScope', () => {
  it('returns the widest scope held', () => {
    const set = PermissionSet.fromRaw([
      'user-management.users.read:own',
      'user-management.users.read:tenant',
    ]);
    expect(set.maxScope('user-management.users.read')).toBe('tenant');
  });

  it('returns null when the permission is not held', () => {
    expect(PermissionSet.fromRaw([]).maxScope('user-management.users.read')).toBeNull();
  });
});

describe('PermissionSet.canDelegate', () => {
  const tenantScoped = PermissionSet.fromRaw(['user-management.users.create:tenant']);

  it('allows granting at the same scope', () => {
    expect(tenantScoped.canDelegate('user-management.users.create', 'tenant')).toBe(true);
  });

  it('allows granting a narrower scope', () => {
    expect(tenantScoped.canDelegate('user-management.users.create', 'own')).toBe(true);
    expect(tenantScoped.canDelegate('user-management.users.create', 'team')).toBe(true);
  });

  it('refuses granting a wider scope', () => {
    expect(tenantScoped.canDelegate('user-management.users.create', 'all')).toBe(false);
  });

  it('refuses granting a permission that is not held', () => {
    expect(tenantScoped.canDelegate('user-management.users.delete', 'own')).toBe(false);
  });

  it('refuses when the permission is denied', () => {
    const denied = new PermissionSet([
      { code: 'user-management.users.create', scope: 'tenant', effect: 'allow' },
      { code: 'user-management.users.create', scope: 'tenant', effect: 'deny' },
    ]);
    expect(denied.canDelegate('user-management.users.create', 'own')).toBe(false);
  });

  it('an own-scoped holder cannot grant tenant scope', () => {
    const ownOnly = PermissionSet.fromRaw(['user-management.users.read:own']);
    expect(ownOnly.canDelegate('user-management.users.read', 'tenant')).toBe(false);
    expect(ownOnly.canDelegate('user-management.users.read', 'own')).toBe(true);
  });
});

describe('PermissionSet.delegatableScopes', () => {
  it('offers only scopes at or below the actor’s own', () => {
    const set = PermissionSet.fromRaw(['user-management.users.read:organization']);
    const scopes = set.delegatableScopes('user-management.users.read', SCOPES);
    expect(scopes).toContain('own');
    expect(scopes).toContain('team');
    expect(scopes).toContain('organization');
    expect(scopes).not.toContain('tenant');
    expect(scopes).not.toContain('all');
  });

  it('offers nothing for a permission that is not held', () => {
    expect(PermissionSet.fromRaw([]).delegatableScopes('user-management.users.read', SCOPES)).toEqual([]);
  });
});

describe('PermissionSet.delegationBlockReason', () => {
  it('explains a missing permission', () => {
    const reason = PermissionSet.fromRaw([]).delegationBlockReason('user-management.users.read', 'own');
    expect(reason).toMatch(/do not hold/i);
  });

  it('explains an explicit deny', () => {
    const denied = new PermissionSet([
      { code: 'user-management.users.read', scope: 'tenant', effect: 'allow' },
      { code: 'user-management.users.read', scope: 'tenant', effect: 'deny' },
    ]);
    expect(denied.delegationBlockReason('user-management.users.read', 'own')).toMatch(/denied/i);
  });

  it('explains a too-wide scope, naming the actor’s own scope', () => {
    const set = PermissionSet.fromRaw(['user-management.users.read:own']);
    const reason = set.delegationBlockReason('user-management.users.read', 'tenant');
    expect(reason).toMatch(/"own"/);
    expect(reason).toMatch(/wider/i);
  });

  it('returns null when the grant is allowed', () => {
    const set = PermissionSet.fromRaw(['user-management.users.read:tenant']);
    expect(set.delegationBlockReason('user-management.users.read', 'own')).toBeNull();
  });
});

describe('PermissionSet.readableFields', () => {
  it('lists only explicitly granted fields', () => {
    const set = PermissionSet.fromRaw([
      'user-management.users.read:tenant',
      'user-management.users.read.email:tenant',
      'user-management.users.read.first_name:tenant',
      'user-management.users.search.phone:tenant',
    ]);
    expect(set.readableFields('user-management', 'users', 'read')).toEqual(['email', 'first_name']);
  });

  it('excludes denied fields', () => {
    const set = new PermissionSet([
      { code: 'user-management.users.read.email', scope: 'tenant', effect: 'allow' },
      { code: 'user-management.users.read.email', scope: 'tenant', effect: 'deny' },
    ]);
    expect(set.readableFields('user-management', 'users', 'read')).toEqual([]);
  });
});
