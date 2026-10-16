/**
 * Plain-language documentation for every permission in the catalogue.
 *
 * The database `description` column is a one-liner meant for lists. This file
 * is the long form shown on hover: what the permission actually lets someone
 * do, how it behaves, and — just as important — what it deliberately does not
 * grant. Someone assigning permissions should be able to decide from the
 * tooltip alone, without reading the code.
 *
 * Keep this in sync with `migrations/0008_seed_catalog.up.sql`. A code with no
 * entry still renders: `permissionDoc` composes a sensible fallback rather than
 * showing nothing.
 *
 * All copy lives here, so translating the tooltips means editing this one file.
 */

export type RiskLevel = 'low' | 'medium' | 'high';

export interface PermissionDoc {
  /** One line, plain language: what holding this lets you do. */
  summary: string;
  /** The mechanics worth knowing before granting it. */
  detail: string;
  /** What this deliberately does NOT grant. Prevents over-granting. */
  caveat?: string;
  risk: RiskLevel;
}

export const RISK_LABELS: Record<RiskLevel, string> = {
  low: 'Low impact',
  medium: 'Elevated',
  high: 'Sensitive',
};

/** What each resource is, used to build fallback text. */
const RESOURCE_LABELS: Record<string, string> = {
  users: 'user accounts',
  tenants: 'tenants (partner companies)',
  roles: 'roles',
  permissions: 'the permission catalogue',
  'api-clients': 'machine-to-machine API clients',
  'audit-logs': 'the audit trail',
  sessions: 'active sign-in sessions',
};

/**
 * Explicit entries for action-level permissions. Field-level permissions
 * (`users.read.email`) are composed from FIELD_DOCS below.
 */
const ACTION_DOCS: Record<string, PermissionDoc> = {
  // ── Users ────────────────────────────────────────────────────────────────
  'users.create': {
    summary: 'Create new user accounts.',
    detail:
      'Creates the account and its credential. The new user always starts with must_change_password = true, so the password you set is single-use. Any protected field you fill in (email, name, phone, status) additionally requires the matching users.update.<field> permission.',
    caveat:
      'Does not let you attach roles the actor does not hold — role assignment during creation goes through the same delegation rule.',
    risk: 'medium',
  },
  'users.read': {
    summary: 'Open a user and see the base fields.',
    detail:
      'Base fields only: id, tenant, user type, must_change_password, timestamps, version. Every protected field is a separate permission, so a user with only this sees a record with the personal details stripped out.',
    caveat: 'Does not reveal email, name, phone, status, internal notes or security flags.',
    risk: 'low',
  },
  'users.list': {
    summary: 'Browse the user list.',
    detail:
      'Returns a page of users inside the scope you hold. Columns follow your read permissions — fields you cannot read are removed from every row, not just hidden in the UI.',
    risk: 'low',
  },
  'users.search': {
    summary: 'Run dynamic searches over users.',
    detail:
      'Lets you call the search endpoint at all. This is stage one of two: every field you filter or sort by is checked separately against users.search.<field>. Holding this alone only lets you search on base fields.',
    caveat: 'Filtering by email or phone needs users.search.email / users.search.phone as well.',
    risk: 'low',
  },
  'users.update': {
    summary: 'Change the base fields of a user.',
    detail:
      'Covers the non-protected fields and version bookkeeping. Each protected field you want to write needs its own users.update.<field> grant; sending a field you cannot write fails the whole request with 403 rather than silently dropping it.',
    risk: 'medium',
  },
  'users.delete': {
    summary: 'Delete a user account.',
    detail:
      'A soft delete: the row stays so the audit trail still resolves who did what, but the account moves to disabled and every one of its sessions is revoked immediately. The email is freed for re-use.',
    caveat: 'Not recoverable through the portal — recreating the person makes a new account with a new id.',
    risk: 'high',
  },

  // ── Tenants ──────────────────────────────────────────────────────────────
  'tenants.create': {
    summary: 'Onboard a new partner company.',
    detail:
      'Creates the tenant and its isolation boundary. Everything that follows — users, API clients, roles, data — lives inside it and cannot be seen from another tenant.',
    caveat: 'Platform-only. A tenant administrator can never hold this, at any scope.',
    risk: 'high',
  },
  'tenants.read': {
    summary: 'View tenant details.',
    detail:
      'Tenant users see their own tenant with this; platform users see any tenant. Row-Level Security enforces the boundary in the database as well, so a missing WHERE clause cannot leak another tenant.',
    risk: 'low',
  },
  'tenants.list': {
    summary: 'Browse all tenants.',
    detail: 'Lists every tenant on the platform — inherently a cross-tenant view.',
    caveat: 'Platform-only.',
    risk: 'medium',
  },
  'tenants.search': {
    summary: 'Run dynamic searches over tenants.',
    detail:
      'Stage one of the two-stage search check: each field you filter or sort by is also checked against tenants.search.<field>.',
    caveat: 'Platform-only.',
    risk: 'medium',
  },
  'tenants.update': {
    summary: 'Change tenant details.',
    detail:
      'Covers name, contact details and status. Suspending a tenant takes effect immediately: its users stop authenticating and its API clients stop receiving tokens.',
    risk: 'high',
  },
  'tenants.delete': {
    summary: 'Terminate a tenant.',
    detail:
      'Moves the tenant to terminated, which is a final state — it cannot be reactivated. Every user and API client under it stops working at once.',
    caveat: 'Platform-only, and irreversible. Suspend instead if you may need to undo it.',
    risk: 'high',
  },

  // ── Roles ────────────────────────────────────────────────────────────────
  'roles.create': {
    summary: 'Create custom roles.',
    detail:
      'Creates an empty role inside your tenant. It starts with no permissions — filling it is a separate step governed by permissions.assign.',
    caveat: 'Cannot create system roles; those are defined by the platform and are immutable.',
    risk: 'medium',
  },
  'roles.read': {
    summary: 'Open a role and see its permissions.',
    detail: 'Shows the role and every grant attached to it, with each grant’s scope and effect.',
    risk: 'low',
  },
  'roles.list': {
    summary: 'Browse the role list.',
    detail:
      'Shows your tenant’s custom roles plus the platform’s system roles, which every tenant can read but nobody can edit.',
    risk: 'low',
  },
  'roles.update': {
    summary: 'Rename a role or change its description.',
    detail:
      'Edits the role’s own attributes. Saving uses optimistic locking: if someone else changed the role since you opened it, the save is rejected with a conflict rather than overwriting their change.',
    caveat:
      'Does not change which permissions the role carries — that needs permissions.assign / permissions.revoke. System roles reject this outright.',
    risk: 'medium',
  },
  'roles.delete': {
    summary: 'Delete a custom role.',
    detail:
      'Soft-deletes the role and detaches it from everyone who held it. Those users immediately lose whatever the role was granting them.',
    caveat: 'System roles cannot be deleted.',
    risk: 'high',
  },
  'roles.assign': {
    summary: 'Give a role to a user.',
    detail:
      'Assigning a role hands over every permission inside it, so the delegation rule applies to the whole set: you can only assign a role whose permissions you hold yourself, at your scope or narrower.',
    caveat: 'Holding this does not let you edit what is inside the role.',
    risk: 'high',
  },
  'roles.revoke': {
    summary: 'Take a role away from a user.',
    detail:
      'Removes the assignment. The user loses those permissions on their next authorization check — cached permissions are invalidated immediately, not on a timer.',
    risk: 'medium',
  },

  // ── Permissions ──────────────────────────────────────────────────────────
  'permissions.list': {
    summary: 'Browse the permission catalogue.',
    detail:
      'Read-only view of every permission the platform defines, as the Service → Resource → Action → Field tree. Needed to render the grant picker.',
    risk: 'low',
  },
  'permissions.read': {
    summary: 'Read a single permission definition.',
    detail: 'Shows one catalogue entry, including whether it is platform-only.',
    risk: 'low',
  },
  'permissions.assign': {
    summary: 'Attach a permission to a role, user or API client.',
    detail:
      'The delegation rule is enforced here and cannot be bypassed: you may only grant a permission you hold yourself, at your own scope or narrower, with conditions at least as tight as yours. Every rejected attempt is written to the audit log.',
    caveat:
      'Platform-only permissions and scope = all are refused on tenant roles regardless of what the actor holds. This is the permission that lets someone widen another person’s access — grant it sparingly.',
    risk: 'high',
  },
  'permissions.revoke': {
    summary: 'Remove a permission from a role, user or API client.',
    detail:
      'Detaches the grant and invalidates cached permissions for everyone affected, so the change is immediate.',
    risk: 'medium',
  },

  // ── API clients ──────────────────────────────────────────────────────────
  'api-clients.create': {
    summary: 'Create a machine-to-machine API client.',
    detail:
      'Issues a client id and a secret for the OAuth2 client-credentials flow. The secret is shown exactly once at creation and stored only as an Argon2id hash — it can never be retrieved again, only rotated.',
    risk: 'medium',
  },
  'api-clients.read': {
    summary: 'View an API client.',
    detail: 'Shows configuration, status, allowed IPs and last-used time.',
    caveat: 'Never shows the secret. No permission exists that reveals it — the plaintext is not stored.',
    risk: 'low',
  },
  'api-clients.list': {
    summary: 'Browse API clients.',
    detail: 'Lists the clients belonging to your tenant.',
    risk: 'low',
  },
  'api-clients.search': {
    summary: 'Run dynamic searches over API clients.',
    detail:
      'Stage one of the two-stage search check: each field you filter or sort by is also checked against api-clients.search.<field>.',
    risk: 'low',
  },
  'api-clients.update': {
    summary: 'Change an API client’s configuration.',
    detail:
      'Covers name, description, allowed IPs and status. Suspending a client stops new tokens being issued immediately.',
    risk: 'medium',
  },
  'api-clients.rotate': {
    summary: 'Issue a new secret for an API client.',
    detail:
      'Generates a replacement secret and kills the previous one the moment the call succeeds. Anything still using the old secret starts failing with 401 straight away.',
    caveat: 'The new secret is displayed once. Losing it means rotating again.',
    risk: 'high',
  },
  'api-clients.delete': {
    summary: 'Revoke an API client.',
    detail: 'Moves the client to revoked. Token requests fail from that point on; existing tokens expire normally.',
    risk: 'high',
  },
  'api-clients.assign': {
    summary: 'Grant a permission to an API client.',
    detail:
      'Same delegation rule as for users: you cannot give a client a permission you do not hold. A client with no grants authenticates successfully and can call nothing.',
    caveat: 'Platform-only permissions and scope = all are always refused for clients.',
    risk: 'high',
  },
  'api-clients.revoke': {
    summary: 'Remove a permission from an API client.',
    detail: 'Detaches the grant; the client loses that access on its next call.',
    risk: 'medium',
  },

  // ── Audit logs ───────────────────────────────────────────────────────────
  'audit-logs.read': {
    summary: 'Open individual audit records.',
    detail:
      'Audit entries are append-only — a database trigger blocks updates and deletes, so what you read is what happened.',
    caveat: 'Sensitive values inside recorded state are redacted; secrets never reach the audit trail.',
    risk: 'low',
  },
  'audit-logs.list': {
    summary: 'Browse the audit trail.',
    detail:
      'Shows security-relevant actions inside your tenant: sign-ins, denials, permission changes, escalation attempts.',
    risk: 'low',
  },
  'audit-logs.search': {
    summary: 'Search the audit trail.',
    detail: 'Filter audit records by actor, action, resource or time window.',
    risk: 'low',
  },
  'audit-logs.export': {
    summary: 'Export audit records in bulk.',
    detail:
      'Produces a downloadable extract. Bulk extraction of who-did-what is normally a compliance function.',
    caveat: 'Consider whether the holder genuinely needs offline copies of the trail.',
    risk: 'medium',
  },

  // ── Sessions ─────────────────────────────────────────────────────────────
  'sessions.read': {
    summary: 'View a sign-in session.',
    detail: 'Shows device, IP and issue time for a refresh-token family.',
    risk: 'low',
  },
  'sessions.list': {
    summary: 'Browse active sessions.',
    detail: 'Lists who is currently signed in within your scope.',
    risk: 'low',
  },
  'sessions.delete': {
    summary: 'Revoke a session and sign someone out.',
    detail:
      'Kills the refresh-token family immediately. The access token already issued stays valid until it expires, at most 15 minutes.',
    caveat: 'The main lever for cutting off a compromised account — but it also disconnects a working user.',
    risk: 'medium',
  },
};

/** What each protected field holds, used to compose field-level entries. */
const FIELD_DOCS: Record<string, { label: string; note: string; risk: RiskLevel }> = {
  email: {
    label: 'email address',
    note: 'Also the sign-in identifier, so changing it changes how the person logs in.',
    risk: 'medium',
  },
  first_name: { label: 'first name', note: 'Ordinary personal data.', risk: 'low' },
  last_name: { label: 'last name', note: 'Ordinary personal data.', risk: 'low' },
  phone: { label: 'phone number', note: 'Personal contact data.', risk: 'medium' },
  status: {
    label: 'account status',
    note: 'Writing it can suspend or reactivate the account, which takes effect on the next request.',
    risk: 'high',
  },
  internal_notes: {
    label: 'internal notes',
    note: 'Staff-only commentary about the person. Not something the user themselves is meant to see.',
    risk: 'high',
  },
  security_flags: {
    label: 'security flags',
    note: 'Internal markers used by the security tooling. Platform-only.',
    risk: 'high',
  },
  name: { label: 'name', note: 'The display name.', risk: 'low' },
  slug: { label: 'slug', note: 'The stable URL-safe identifier.', risk: 'low' },
  client_id: { label: 'client id', note: 'The public half of the client credentials.', risk: 'low' },
};

const ACTION_VERBS: Record<string, string> = {
  read: 'See',
  update: 'Change',
  search: 'Filter and sort by',
  create: 'Set on creation',
};

/**
 * Look up the documentation for a permission code such as `users.read.email`.
 * Never returns undefined — an unknown code gets a composed fallback so a newly
 * seeded permission is still explained rather than rendering a blank tooltip.
 */
export function permissionDoc(code: string): PermissionDoc {
  const explicit = ACTION_DOCS[code];
  if (explicit) return explicit;

  const parts = code.split('.');
  const [resource, action, ...rest] = parts;
  const field = rest.join('.');

  if (field) {
    const info = FIELD_DOCS[field];
    const verb = ACTION_VERBS[action] ?? action;
    const what = info?.label ?? field.replace(/_/g, ' ');

    if (action === 'search') {
      return {
        summary: `Filter and sort by ${what}.`,
        detail: `Stage two of the search check. Without this, a query that touches ${what} is rejected with 403 — the field is not silently ignored, so a search never quietly returns the wrong answer.${info ? ` ${info.note}` : ''}`,
        caveat: `Needs ${resource}.search as well, which is what allows calling the endpoint at all.`,
        risk: info?.risk ?? 'low',
      };
    }

    if (action === 'read') {
      return {
        summary: `See the ${what}.`,
        detail: `Without this the field is stripped from every response — lists, detail pages and search results alike.${info ? ` ${info.note}` : ''}`,
        risk: info?.risk ?? 'low',
      };
    }

    if (action === 'update') {
      return {
        summary: `Change the ${what}.`,
        detail: `Sending this field without the permission fails the whole request with 403 rather than dropping the field.${info ? ` ${info.note}` : ''}`,
        caveat: `Needs ${resource}.update as well.`,
        risk: info?.risk ?? 'medium',
      };
    }

    return {
      summary: `${verb} the ${what}.`,
      detail: info?.note ?? `Field-level permission on ${what}.`,
      risk: info?.risk ?? 'medium',
    };
  }

  const subject = RESOURCE_LABELS[resource] ?? resource;
  return {
    summary: `${action.charAt(0).toUpperCase() + action.slice(1)} ${subject}.`,
    detail:
      'No extended description is recorded for this permission yet. It follows the standard rules: deny beats allow, and it is limited by the scope it was granted at.',
    risk: 'medium',
  };
}

/** Whether the tooltip should carry a visible warning stripe. */
export function isSensitive(code: string): boolean {
  return permissionDoc(code).risk === 'high';
}
