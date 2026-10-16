# Wallet Gateway request authorization policy.
#
# Responsibility split:
#   * OpenFGA  — relationship/ownership graph (checked by the service before
#                calling OPA; result arrives as input.relationships).
#   * OPA (here) — request-context and business policy: tenant boundary,
#                  permission matching with scope/effect, field-level output.
#   * Domain engine — re-verifies the action permission (defence in depth).
#
# Deny by default: `allow` is false unless a rule grants it.

package wallet.authz

import rego.v1

# ── Entry point ─────────────────────────────────────────────────────────────

decision := {
	"allow": allow,
	"reason": reason,
	"allowed_fields": allowed_fields,
	"denied_fields": denied_fields,
	"filters": filters,
}

default allow := false

allow if {
	not cross_tenant_violation
	not denied_by_explicit_deny
	some p in matching_action_permissions
	scope_reaches(p.scope)
}

# ── Reasons (first match wins, most specific first) ─────────────────────────

default reason := "permission_denied"

reason := "cross_tenant_denied" if cross_tenant_violation

reason := "explicit_deny" if {
	not cross_tenant_violation
	denied_by_explicit_deny
}

reason := "permission_missing" if {
	not cross_tenant_violation
	not denied_by_explicit_deny
	count(matching_action_permissions) == 0
}

reason := "scope_insufficient" if {
	not cross_tenant_violation
	not denied_by_explicit_deny
	count(matching_action_permissions) > 0
	not any_scope_reaches
}

reason := "permission_granted" if allow

# ── Tenant boundary ─────────────────────────────────────────────────────────

platform_principal if input.principal.type == "platform_user"

platform_principal if input.principal.type == "system_service"

# A non-platform principal may never touch another tenant's resources.
cross_tenant_violation if {
	not platform_principal
	input.target.tenant_id != ""
	input.target.tenant_id != input.principal.tenant_id
}

# ── Permission matching ─────────────────────────────────────────────────────

# The action-level code the request needs, e.g.
# "user-management.users.create".
required_code := sprintf("user-management.%s.%s", [input.request.resource, input.request.action])

allow_permissions contains p if {
	some p in input.permissions
	p.effect == "allow"
}

deny_permissions contains p if {
	some p in input.permissions
	p.effect == "deny"
}

matching_action_permissions contains p if {
	some p in allow_permissions
	p.code == required_code
}

# An explicit deny on the exact action code blocks the request when its scope
# reaches the target. Deny always beats allow.
denied_by_explicit_deny if {
	some p in deny_permissions
	p.code == required_code
	scope_reaches(p.scope)
}

any_scope_reaches if {
	some p in matching_action_permissions
	scope_reaches(p.scope)
}

# ── Scope evaluation ────────────────────────────────────────────────────────

scope_reaches("all")

scope_reaches("tenant") if input.relationships.same_tenant

scope_reaches("tenant") if input.relationships.is_self

scope_reaches("descendants") if input.relationships.same_tenant

scope_reaches("organization") if input.relationships.same_tenant

scope_reaches("team") if input.relationships.same_tenant

scope_reaches("assigned") if input.relationships.is_self

scope_reaches("own") if input.relationships.is_self

# Creates and lists have no concrete target row: a tenant-wide grant reaches
# them inside the principal's own tenant.
scope_reaches("tenant") if collection_request

scope_reaches("organization") if collection_request

scope_reaches("team") if collection_request

collection_request if {
	input.request.action in {"create", "list", "search"}
	not cross_tenant_violation
}

# ── Field-level output ──────────────────────────────────────────────────────

# Fields the principal explicitly holds for this resource+action, minus any
# field the principal is explicitly denied.
allowed_fields := sort([field |
	some p in allow_permissions
	field := field_of(p.code)
	field != ""
	scope_reaches(p.scope)
	not field_denied(field)
])

denied_fields := sort([field |
	some p in deny_permissions
	field := field_of(p.code)
	field != ""
])

field_denied(field) if {
	some p in deny_permissions
	field_of(p.code) == field
}

# An action-level deny also denies every field of that action.
field_denied(_) if {
	some p in deny_permissions
	p.code == required_code
}

# field_of extracts the 4th segment of
# "<service>.<resource>.<action>.<field>" when it belongs to this request.
field_of(code) := field if {
	parts := split(code, ".")
	count(parts) == 4
	parts[1] == input.request.resource
	parts[2] == input.request.action
	field := parts[3]
} else := ""

# ── Row filters ─────────────────────────────────────────────────────────────

# Tenant principals are always constrained to their own tenant; the service
# applies this on top of PostgreSQL RLS.
filters := {"tenant_id": input.principal.tenant_id} if {
	not platform_principal
	input.principal.tenant_id != ""
} else := {}
