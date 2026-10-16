import { SCOPE_RANK, type Effect, type Scope } from '../api/types';

/**
 * Client-side mirror of the backend permission model.
 *
 * This exists to shape the UI — hide menus, disable buttons, grey out scopes
 * the actor cannot delegate. It is NOT a security boundary: the API re-runs
 * every check (OpenFGA relationship + OPA policy + domain engine) and rejects
 * anything this layer would have allowed by mistake. Treat divergence here as
 * a UX bug, never as a vulnerability.
 */

export interface HeldPermission {
  code: string; // service.resource.action[.field]
  scope: Scope;
  effect: Effect;
}

/** Parses "user-management.users.read.email:tenant" from GET /auth/me. */
export function parsePermission(raw: string): HeldPermission | null {
  const [code, scope] = raw.split(':');
  if (!code || !scope) return null;
  if (!(scope in SCOPE_RANK)) return null;
  return { code, scope: scope as Scope, effect: 'allow' };
}

export function parsePermissions(raw: string[]): HeldPermission[] {
  return raw.map(parsePermission).filter((p): p is HeldPermission => p !== null);
}

export class PermissionSet {
  private readonly byCode: Map<string, HeldPermission[]>;

  constructor(permissions: HeldPermission[]) {
    this.byCode = new Map();
    for (const p of permissions) {
      const list = this.byCode.get(p.code) ?? [];
      list.push(p);
      this.byCode.set(p.code, list);
    }
  }

  static fromRaw(raw: string[]): PermissionSet {
    return new PermissionSet(parsePermissions(raw));
  }

  /** True when the actor holds the permission at any scope (deny wins). */
  has(code: string): boolean {
    const entries = this.byCode.get(code);
    if (!entries || entries.length === 0) return false;
    if (entries.some((e) => e.effect === 'deny')) return false;
    return entries.some((e) => e.effect === 'allow');
  }

  /** True when the actor holds any of the codes. */
  hasAny(...codes: string[]): boolean {
    return codes.some((c) => this.has(c));
  }

  /** The widest scope held for a code, or null when not held. */
  maxScope(code: string): Scope | null {
    const entries = this.byCode.get(code)?.filter((e) => e.effect === 'allow') ?? [];
    if (entries.length === 0) return null;
    return entries.reduce<Scope>(
      (widest, e) => (SCOPE_RANK[e.scope] > SCOPE_RANK[widest] ? e.scope : widest),
      entries[0].scope,
    );
  }

  /**
   * Mirrors the backend delegation rule: a grant must be an equal-or-narrower
   * subset of a held permission (same code, scope no wider).
   */
  canDelegate(code: string, scope: Scope): boolean {
    if (!this.has(code)) return false;
    const widest = this.maxScope(code);
    if (!widest) return false;
    return SCOPE_RANK[widest] >= SCOPE_RANK[scope];
  }

  /** Scopes the actor may grant for a code (for scope pickers). */
  delegatableScopes(code: string, candidates: Scope[]): Scope[] {
    const widest = this.maxScope(code);
    if (!widest) return [];
    return candidates.filter((s) => SCOPE_RANK[s] <= SCOPE_RANK[widest]);
  }

  /** Explains why a permission cannot be delegated (for disabled tooltips). */
  delegationBlockReason(code: string, scope: Scope): string | null {
    if (!this.byCode.has(code)) return 'You do not hold this permission, so you cannot grant it.';
    const entries = this.byCode.get(code) ?? [];
    if (entries.some((e) => e.effect === 'deny')) return 'This permission is explicitly denied for you.';
    const widest = this.maxScope(code);
    if (!widest) return 'You do not hold this permission, so you cannot grant it.';
    if (SCOPE_RANK[widest] < SCOPE_RANK[scope]) {
      return `Your own scope for this permission is "${widest}"; you cannot grant a wider scope.`;
    }
    return null;
  }

  /** Fields readable for resource+action, derived from field-level codes. */
  readableFields(service: string, resource: string, action: string): string[] {
    const prefix = `${service}.${resource}.${action}.`;
    const fields: string[] = [];
    for (const [code, entries] of this.byCode) {
      if (!code.startsWith(prefix)) continue;
      if (entries.some((e) => e.effect === 'deny')) continue;
      fields.push(code.slice(prefix.length));
    }
    return fields.sort();
  }

  get size(): number {
    return this.byCode.size;
  }
}

// ─── Permission codes used by the UI ────────────────────────────────────────

const S = 'user-management';

export const P = {
  tenants: {
    create: `${S}.tenants.create`,
    read: `${S}.tenants.read`,
    update: `${S}.tenants.update`,
    delete: `${S}.tenants.delete`,
    list: `${S}.tenants.list`,
  },
  users: {
    create: `${S}.users.create`,
    read: `${S}.users.read`,
    update: `${S}.users.update`,
    delete: `${S}.users.delete`,
    list: `${S}.users.list`,
    search: `${S}.users.search`,
    readField: (field: string) => `${S}.users.read.${field}`,
    updateField: (field: string) => `${S}.users.update.${field}`,
    searchField: (field: string) => `${S}.users.search.${field}`,
  },
  roles: {
    create: `${S}.roles.create`,
    read: `${S}.roles.read`,
    update: `${S}.roles.update`,
    delete: `${S}.roles.delete`,
    list: `${S}.roles.list`,
    assign: `${S}.roles.assign`,
    revoke: `${S}.roles.revoke`,
  },
  permissions: {
    read: `${S}.permissions.read`,
    list: `${S}.permissions.list`,
    assign: `${S}.permissions.assign`,
    revoke: `${S}.permissions.revoke`,
  },
  apiClients: {
    create: `${S}.api-clients.create`,
    read: `${S}.api-clients.read`,
    update: `${S}.api-clients.update`,
    delete: `${S}.api-clients.delete`,
    list: `${S}.api-clients.list`,
    rotate: `${S}.api-clients.rotate`,
    assign: `${S}.api-clients.assign`,
  },
  auditLogs: {
    read: `${S}.audit-logs.read`,
    list: `${S}.audit-logs.list`,
    export: `${S}.audit-logs.export`,
  },
  sessions: {
    read: `${S}.sessions.read`,
    list: `${S}.sessions.list`,
    delete: `${S}.sessions.delete`,
  },
} as const;

/** User fields that require an explicit field-level grant. */
export const USER_SEARCHABLE_FIELDS = ['email', 'first_name', 'last_name', 'phone', 'status'] as const;
export const USER_EDITABLE_FIELDS = ['email', 'first_name', 'last_name', 'phone', 'internal_notes'] as const;
