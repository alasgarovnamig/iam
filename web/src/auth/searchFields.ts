import type { SearchField } from '../api/search';
import type { PermissionSet } from './permissions';

/**
 * Searchable field catalogs, mirroring the backend resource schemas
 * (domain/search.go). Each field maps to a `<resource>.search.<field>`
 * permission the actor must hold to filter by it. `base` fields (created_at)
 * need only the action-level `<resource>.search`.
 */

export const USER_SEARCH_FIELDS: SearchField[] = [
  { name: 'email', label: 'Email', type: 'text' },
  { name: 'first_name', label: 'First name', type: 'text' },
  { name: 'last_name', label: 'Last name', type: 'text' },
  { name: 'phone', label: 'Phone', type: 'text' },
  { name: 'status', label: 'Status', type: 'enum', options: ['pending', 'active', 'suspended', 'locked', 'disabled'] },
  { name: 'created_at', label: 'Created', type: 'timestamp', base: true },
];

export const TENANT_SEARCH_FIELDS: SearchField[] = [
  { name: 'name', label: 'Name', type: 'text' },
  { name: 'slug', label: 'Slug', type: 'text' },
  { name: 'status', label: 'Status', type: 'enum', options: ['pending', 'active', 'suspended', 'terminated'] },
  { name: 'created_at', label: 'Created', type: 'timestamp', base: true },
];

export const API_CLIENT_SEARCH_FIELDS: SearchField[] = [
  { name: 'name', label: 'Name', type: 'text' },
  { name: 'client_id', label: 'Client ID', type: 'text' },
  { name: 'status', label: 'Status', type: 'enum', options: ['active', 'suspended', 'expired', 'revoked'] },
  { name: 'created_at', label: 'Created', type: 'timestamp', base: true },
];

export function searchActionCode(resource: string): string {
  return `user-management.${resource}.search`;
}

export function searchFieldCode(resource: string, field: string): string {
  return `user-management.${resource}.search.${field}`;
}

/**
 * The fields the actor may actually search: it must hold the action-level
 * search permission, and for non-base fields the per-field grant. This is what
 * makes the builder permission-aware — you cannot even select a field you have
 * no right to filter by. (The backend re-checks regardless.)
 */
export function searchableFields(
  permissions: PermissionSet,
  resource: string,
  all: SearchField[],
): SearchField[] {
  if (!permissions.has(searchActionCode(resource))) return [];
  return all.filter((f) => f.base || permissions.has(searchFieldCode(resource, f.name)));
}
