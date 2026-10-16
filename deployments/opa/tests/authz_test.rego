package wallet.authz_test

import data.wallet.authz
import rego.v1

# ── Fixtures ────────────────────────────────────────────────────────────────

tenant_a := "11111111-1111-4111-8111-111111111111"

tenant_b := "22222222-2222-4222-8222-222222222222"

tenant_user_input(perms, action, resource, target_tenant, rels) := {
	"principal": {"id": "user-1", "type": "tenant_user", "tenant_id": tenant_a},
	"request": {"method": "POST", "path": "/api/v1/users", "action": action, "resource": resource},
	"target": {"tenant_id": target_tenant, "resource_id": ""},
	"permissions": perms,
	"relationships": rels,
	"context": {"ip": "127.0.0.1", "time": "2026-01-01T00:00:00Z"},
}

perm(code, scope, effect) := {"code": code, "scope": scope, "effect": effect}

same_tenant_rels := {"is_self": false, "same_tenant": true, "platform": false, "member": true}

self_rels := {"is_self": true, "same_tenant": true, "platform": false, "member": true}

other_tenant_rels := {"is_self": false, "same_tenant": false, "platform": false, "member": true}

# ── Deny by default ─────────────────────────────────────────────────────────

test_deny_by_default_with_no_permissions if {
	decision := authz.decision with input as tenant_user_input([], "create", "users", tenant_a, same_tenant_rels)
	decision.allow == false
	decision.reason == "permission_missing"
}

# ── Basic allow ─────────────────────────────────────────────────────────────

test_allow_create_with_tenant_scope if {
	perms := [perm("user-management.users.create", "tenant", "allow")]
	decision := authz.decision with input as tenant_user_input(perms, "create", "users", tenant_a, same_tenant_rels)
	decision.allow == true
	decision.reason == "permission_granted"
}

# ── Tenant isolation ────────────────────────────────────────────────────────

test_cross_tenant_denied_even_with_permission if {
	perms := [perm("user-management.users.read", "tenant", "allow")]
	decision := authz.decision with input as tenant_user_input(perms, "read", "users", tenant_b, other_tenant_rels)
	decision.allow == false
	decision.reason == "cross_tenant_denied"
}

test_platform_user_may_cross_tenants if {
	perms := [perm("user-management.users.read", "all", "allow")]
	req := {
		"principal": {"id": "admin-1", "type": "platform_user", "tenant_id": ""},
		"request": {"method": "GET", "path": "/api/v1/users/x", "action": "read", "resource": "users"},
		"target": {"tenant_id": tenant_b, "resource_id": "x"},
		"permissions": perms,
		"relationships": {"is_self": false, "same_tenant": true, "platform": true, "member": true},
		"context": {"ip": "127.0.0.1", "time": "2026-01-01T00:00:00Z"},
	}
	decision := authz.decision with input as req
	decision.allow == true
}

# ── Deny beats allow ────────────────────────────────────────────────────────

test_explicit_deny_overrides_allow if {
	perms := [
		perm("user-management.users.delete", "tenant", "allow"),
		perm("user-management.users.delete", "tenant", "deny"),
	]
	decision := authz.decision with input as tenant_user_input(perms, "delete", "users", tenant_a, same_tenant_rels)
	decision.allow == false
	decision.reason == "explicit_deny"
}

# ── Scope evaluation ────────────────────────────────────────────────────────

test_own_scope_does_not_reach_other_user if {
	perms := [perm("user-management.users.read", "own", "allow")]
	decision := authz.decision with input as tenant_user_input(perms, "read", "users", tenant_a, same_tenant_rels)
	decision.allow == false
	decision.reason == "scope_insufficient"
}

test_own_scope_reaches_self if {
	perms := [perm("user-management.users.read", "own", "allow")]
	decision := authz.decision with input as tenant_user_input(perms, "read", "users", tenant_a, self_rels)
	decision.allow == true
}

# ── Field-level output ──────────────────────────────────────────────────────

test_allowed_fields_only_lists_granted_fields if {
	perms := [
		perm("user-management.users.read", "tenant", "allow"),
		perm("user-management.users.read.email", "tenant", "allow"),
		perm("user-management.users.read.first_name", "tenant", "allow"),
	]
	decision := authz.decision with input as tenant_user_input(perms, "read", "users", tenant_a, same_tenant_rels)
	decision.allow == true
	decision.allowed_fields == ["email", "first_name"]
}

test_phone_absent_when_not_granted if {
	perms := [
		perm("user-management.users.read", "tenant", "allow"),
		perm("user-management.users.read.email", "tenant", "allow"),
	]
	decision := authz.decision with input as tenant_user_input(perms, "read", "users", tenant_a, same_tenant_rels)
	not "phone" in decision.allowed_fields
}

test_field_deny_removes_field_from_allowed if {
	perms := [
		perm("user-management.users.read", "tenant", "allow"),
		perm("user-management.users.read.email", "tenant", "allow"),
		perm("user-management.users.read.email", "tenant", "deny"),
	]
	decision := authz.decision with input as tenant_user_input(perms, "read", "users", tenant_a, same_tenant_rels)
	not "email" in decision.allowed_fields
	"email" in decision.denied_fields
}

test_fields_of_other_actions_are_ignored if {
	perms := [
		perm("user-management.users.read", "tenant", "allow"),
		perm("user-management.users.read.email", "tenant", "allow"),
		perm("user-management.users.search.phone", "tenant", "allow"),
	]
	decision := authz.decision with input as tenant_user_input(perms, "read", "users", tenant_a, same_tenant_rels)
	decision.allowed_fields == ["email"]
}

# ── Row filters ─────────────────────────────────────────────────────────────

test_tenant_filter_applied_for_tenant_principal if {
	perms := [perm("user-management.users.list", "tenant", "allow")]
	decision := authz.decision with input as tenant_user_input(perms, "list", "users", tenant_a, same_tenant_rels)
	decision.filters.tenant_id == tenant_a
}

test_no_filter_for_platform_principal if {
	perms := [perm("user-management.users.list", "all", "allow")]
	req := {
		"principal": {"id": "admin-1", "type": "platform_user", "tenant_id": ""},
		"request": {"method": "GET", "path": "/api/v1/users", "action": "list", "resource": "users"},
		"target": {"tenant_id": "", "resource_id": ""},
		"permissions": perms,
		"relationships": {"is_self": false, "same_tenant": false, "platform": true, "member": true},
		"context": {"ip": "127.0.0.1", "time": "2026-01-01T00:00:00Z"},
	}
	decision := authz.decision with input as req
	decision.filters == {}
}

# ── Wrong resource/action never leaks ───────────────────────────────────────

test_permission_for_other_resource_does_not_grant if {
	perms := [perm("user-management.roles.create", "tenant", "allow")]
	decision := authz.decision with input as tenant_user_input(perms, "create", "users", tenant_a, same_tenant_rels)
	decision.allow == false
}

test_permission_for_other_action_does_not_grant if {
	perms := [perm("user-management.users.read", "tenant", "allow")]
	decision := authz.decision with input as tenant_user_input(perms, "delete", "users", tenant_a, same_tenant_rels)
	decision.allow == false
}
