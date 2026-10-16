/**
 * Domain types mirroring the API contract (openapi.yaml).
 *
 * `npm run generate:api` regenerates `schema.ts` straight from the OpenAPI
 * document; the aliases below bind these hand-written names to the generated
 * schema so a contract change breaks the build instead of leaking into
 * runtime. Types the generator cannot express (permission-filtered user
 * fields) stay explicit here.
 */
import type { components } from './schema';

export type Tenant = components['schemas']['Tenant'];
export type Role = components['schemas']['Role'];
export type PermissionDefinition = components['schemas']['PermissionDefinition'];
export type PermissionGrant = components['schemas']['PermissionGrant'];
export type ApiClient = components['schemas']['ApiClient'];
export type AuditLog = components['schemas']['AuditLog'];
export type Session = components['schemas']['Session'];

/**
 * A user as returned by the API. Every field the caller lacks read permission
 * for is ABSENT from the payload — hence all optional. Never assume presence;
 * use `has(user, 'email')` at the call site.
 */
export interface User {
  id: string;
  tenant_id: string;
  user_type: 'platform_user' | 'tenant_user' | 'service_account';
  must_change_password: boolean;
  created_at: string;
  updated_at: string;
  version: number;
  email?: string;
  first_name?: string;
  last_name?: string;
  phone?: string;
  status?: UserStatus;
  internal_notes?: string;
  security_flags?: Record<string, unknown>;
}

export type UserStatus = 'pending' | 'active' | 'suspended' | 'locked' | 'disabled';
export type TenantStatus = 'pending' | 'active' | 'suspended' | 'terminated';
export type ApiClientStatus = 'active' | 'suspended' | 'expired' | 'revoked';

export type Scope = 'own' | 'assigned' | 'team' | 'organization' | 'descendants' | 'tenant' | 'all';
export type Effect = 'allow' | 'deny';

export const SCOPES: Scope[] = ['own', 'assigned', 'team', 'organization', 'descendants', 'tenant', 'all'];

/** Widening order used to disable scopes the actor cannot delegate. */
export const SCOPE_RANK: Record<Scope, number> = {
  own: 1,
  assigned: 1,
  team: 2,
  organization: 3,
  descendants: 4,
  tenant: 5,
  all: 6,
};

export const SCOPE_LABELS: Record<Scope, string> = {
  own: 'Own',
  assigned: 'Assigned',
  team: 'Team',
  organization: 'Organization',
  descendants: 'Descendants',
  tenant: 'Tenant',
  all: 'All (platform-wide)',
};

export interface LoginResponse {
  access_token: string;
  token_type: string;
  expires_at: string;
  must_change_password: boolean;
  user: User;
  /** Present only for non-browser clients; the portal relies on the cookie. */
  refresh_token?: string;
}

export interface MeResponse {
  user: User;
  /** Effective permissions as "service.resource.action[.field]:scope". */
  permissions: string[];
}

/** Permission catalog rendered as a tree by GET /permissions/tree. */
export interface PermissionTreeAction {
  id: string;
  action: string;
  code: string;
  fields?: PermissionDefinition[];
}

export interface PermissionTreeResource {
  resource: string;
  actions: PermissionTreeAction[];
}

export interface PermissionTreeService {
  service: string;
  resources: PermissionTreeResource[];
}

export interface CreatedApiClient {
  client: ApiClient;
  /** Shown exactly once, at creation and rotation. */
  client_secret: string;
  warning: string;
}

export interface RoleWithPermissions {
  role: Role;
  permissions: PermissionGrant[];
}

export interface ApiClientWithPermissions {
  client: ApiClient;
  permissions: PermissionGrant[];
}

/** Type guard for permission-filtered fields. */
export function has<K extends keyof User>(user: User, field: K): user is User & Required<Pick<User, K>> {
  return user[field] !== undefined;
}

export function displayName(user: User): string {
  const name = [user.first_name, user.last_name].filter(Boolean).join(' ').trim();
  if (name) return name;
  return user.email ?? user.id.slice(0, 8);
}
