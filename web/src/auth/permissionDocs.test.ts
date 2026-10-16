import { describe, expect, it } from 'vitest';
import { permissionDoc, isSensitive } from './permissionDocs';

/**
 * Every permission seeded by migrations/0008_seed_catalog.up.sql (plus the
 * search fields added by 0009). If a migration adds a permission, add it here —
 * the coverage test below is what stops a new permission shipping with an empty
 * tooltip.
 */
const CATALOGUE = [
  'api-clients.assign',
  'api-clients.create',
  'api-clients.delete',
  'api-clients.list',
  'api-clients.read',
  'api-clients.revoke',
  'api-clients.rotate',
  'api-clients.search',
  'api-clients.search.client_id',
  'api-clients.search.name',
  'api-clients.search.status',
  'api-clients.update',
  'audit-logs.export',
  'audit-logs.list',
  'audit-logs.read',
  'audit-logs.search',
  'permissions.assign',
  'permissions.list',
  'permissions.read',
  'permissions.revoke',
  'roles.assign',
  'roles.create',
  'roles.delete',
  'roles.list',
  'roles.read',
  'roles.revoke',
  'roles.update',
  'sessions.delete',
  'sessions.list',
  'sessions.read',
  'tenants.create',
  'tenants.delete',
  'tenants.list',
  'tenants.read',
  'tenants.search',
  'tenants.search.name',
  'tenants.search.slug',
  'tenants.search.status',
  'tenants.update',
  'users.create',
  'users.delete',
  'users.list',
  'users.read',
  'users.read.email',
  'users.read.first_name',
  'users.read.internal_notes',
  'users.read.last_name',
  'users.read.phone',
  'users.read.security_flags',
  'users.read.status',
  'users.search',
  'users.search.email',
  'users.search.first_name',
  'users.search.last_name',
  'users.search.phone',
  'users.search.status',
  'users.update',
  'users.update.email',
  'users.update.first_name',
  'users.update.internal_notes',
  'users.update.last_name',
  'users.update.phone',
  'users.update.status',
];

const GENERIC_FALLBACK = 'No extended description is recorded';

describe('permissionDoc', () => {
  it('covers the whole catalogue with usable copy', () => {
    expect(CATALOGUE).toHaveLength(63);
    for (const code of CATALOGUE) {
      const doc = permissionDoc(code);
      expect(doc.summary, code).toBeTruthy();
      expect(doc.detail, code).toBeTruthy();
      // Nothing in the catalogue should land on the "no description" fallback.
      expect(doc.detail, code).not.toContain(GENERIC_FALLBACK);
    }
  });

  it('explains that field search needs both stages', () => {
    const doc = permissionDoc('users.search.email');
    expect(doc.summary).toContain('email');
    expect(doc.caveat).toContain('users.search');
  });

  it('warns that an unwritable field fails the whole request', () => {
    const doc = permissionDoc('users.update.phone');
    expect(doc.detail).toContain('403');
    expect(doc.caveat).toContain('users.update');
  });

  it('marks account-status and internal-notes fields as sensitive', () => {
    expect(permissionDoc('users.update.status').risk).toBe('high');
    expect(permissionDoc('users.read.internal_notes').risk).toBe('high');
    expect(permissionDoc('users.read.first_name').risk).toBe('low');
  });

  it('flags the permissions that widen other people access as sensitive', () => {
    expect(isSensitive('permissions.assign')).toBe(true);
    expect(isSensitive('roles.assign')).toBe(true);
    expect(isSensitive('tenants.delete')).toBe(true);
    expect(isSensitive('api-clients.rotate')).toBe(true);
  });

  it('notes that platform-only permissions cannot reach a tenant admin', () => {
    expect(permissionDoc('tenants.create').caveat).toContain('Platform-only');
    expect(permissionDoc('tenants.delete').caveat).toContain('Platform-only');
  });

  it('states that an API client secret is never readable', () => {
    expect(permissionDoc('api-clients.read').caveat).toContain('Never shows the secret');
  });

  it('falls back gracefully for a permission that is not documented yet', () => {
    const doc = permissionDoc('widgets.frobnicate');
    expect(doc.summary).toBe('Frobnicate widgets.');
    expect(doc.detail).toContain(GENERIC_FALLBACK);
    expect(doc.risk).toBe('medium');
  });

  it('composes a field fallback for an undocumented field', () => {
    const doc = permissionDoc('users.read.nickname');
    expect(doc.summary).toBe('See the nickname.');
    expect(doc.detail).toContain('stripped from every response');
  });
});
