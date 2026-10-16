package domain

import (
	"testing"
)

// perm is a terse constructor for table entries.
func perm(resource string, action Action, field string, scope Scope, effect Effect) Permission {
	return Permission{
		Service: "user-management", Resource: resource, Action: action,
		Field: field, Scope: scope, Effect: effect,
	}
}

func permCond(resource string, action Action, field string, scope Scope, effect Effect, conds map[string]string) Permission {
	p := perm(resource, action, field, scope, effect)
	p.Conditions = conds
	return p
}

// ─── The delegation rule ────────────────────────────────────────────────────

// TestCanDelegate is the table-driven contract for the core business rule:
// a principal may only grant a permission that is an equal-or-narrower subset
// of one it holds.
func TestCanDelegate(t *testing.T) {
	tests := []struct {
		name    string
		held    []Permission
		grant   Permission
		wantErr bool
		reason  string // substring expected in the rejection reason
	}{
		{
			name:  "grants a permission it holds at the same scope",
			held:  []Permission{perm("users", ActionCreate, "", ScopeTenant, EffectAllow)},
			grant: perm("users", ActionCreate, "", ScopeTenant, EffectAllow),
		},
		{
			name:  "narrows the scope when granting (tenant -> own)",
			held:  []Permission{perm("users", ActionCreate, "", ScopeTenant, EffectAllow)},
			grant: perm("users", ActionCreate, "", ScopeOwn, EffectAllow),
		},
		{
			name:    "cannot grant a permission it does not hold",
			held:    []Permission{perm("users", ActionCreate, "", ScopeTenant, EffectAllow)},
			grant:   perm("users", ActionDelete, "", ScopeTenant, EffectAllow),
			wantErr: true,
			reason:  "you do not hold this permission",
		},
		{
			name:    "scope=own holder cannot grant scope=tenant",
			held:    []Permission{perm("users", ActionRead, "", ScopeOwn, EffectAllow)},
			grant:   perm("users", ActionRead, "", ScopeTenant, EffectAllow),
			wantErr: true,
			reason:  "wider than your scope",
		},
		{
			name:    "scope=tenant holder cannot grant scope=all",
			held:    []Permission{perm("users", ActionRead, "", ScopeTenant, EffectAllow)},
			grant:   perm("users", ActionRead, "", ScopeAll, EffectAllow),
			wantErr: true,
			reason:  "wider than your scope",
		},
		{
			name:  "read.email holder can grant read.email",
			held:  []Permission{perm("users", ActionRead, "email", ScopeTenant, EffectAllow)},
			grant: perm("users", ActionRead, "email", ScopeTenant, EffectAllow),
		},
		{
			name:    "read.email holder cannot grant read.phone",
			held:    []Permission{perm("users", ActionRead, "email", ScopeTenant, EffectAllow)},
			grant:   perm("users", ActionRead, "phone", ScopeTenant, EffectAllow),
			wantErr: true,
			reason:  "you do not hold this permission",
		},
		{
			name:    "action-level grant does not imply a field-level grant",
			held:    []Permission{perm("users", ActionRead, "", ScopeTenant, EffectAllow)},
			grant:   perm("users", ActionRead, "email", ScopeTenant, EffectAllow),
			wantErr: true,
			reason:  "you do not hold this permission",
		},
		{
			name:    "field-level grant does not imply the action-level grant",
			held:    []Permission{perm("users", ActionRead, "email", ScopeTenant, EffectAllow)},
			grant:   perm("users", ActionRead, "", ScopeTenant, EffectAllow),
			wantErr: true,
			reason:  "you do not hold this permission",
		},
		{
			name: "an explicit deny blocks delegating that permission",
			held: []Permission{
				perm("users", ActionDelete, "", ScopeTenant, EffectAllow),
				perm("users", ActionDelete, "", ScopeTenant, EffectDeny),
			},
			grant:   perm("users", ActionDelete, "", ScopeOwn, EffectAllow),
			wantErr: true,
			reason:  "explicitly denied",
		},
		{
			name: "an action-level deny blocks delegating its fields",
			held: []Permission{
				perm("users", ActionRead, "email", ScopeTenant, EffectAllow),
				perm("users", ActionRead, "", ScopeTenant, EffectDeny),
			},
			grant:   perm("users", ActionRead, "email", ScopeTenant, EffectAllow),
			wantErr: true,
			reason:  "explicitly denied",
		},
		{
			name:    "conditions cannot be dropped when delegating",
			held:    []Permission{permCond("users", ActionRead, "", ScopeTenant, EffectAllow, map[string]string{"department": "support"})},
			grant:   perm("users", ActionRead, "", ScopeTenant, EffectAllow),
			wantErr: true,
			reason:  "less restrictive",
		},
		{
			name:  "conditions may be kept identical",
			held:  []Permission{permCond("users", ActionRead, "", ScopeTenant, EffectAllow, map[string]string{"department": "support"})},
			grant: permCond("users", ActionRead, "", ScopeTenant, EffectAllow, map[string]string{"department": "support"}),
		},
		{
			name: "conditions may be tightened further when delegating",
			held: []Permission{permCond("users", ActionRead, "", ScopeTenant, EffectAllow, map[string]string{"department": "support"})},
			grant: permCond("users", ActionRead, "", ScopeTenant, EffectAllow,
				map[string]string{"department": "support", "region": "az"}),
		},
		{
			name:    "conditions cannot be changed to a different value",
			held:    []Permission{permCond("users", ActionRead, "", ScopeTenant, EffectAllow, map[string]string{"department": "support"})},
			grant:   permCond("users", ActionRead, "", ScopeTenant, EffectAllow, map[string]string{"department": "finance"}),
			wantErr: true,
			reason:  "less restrictive",
		},
		{
			name:    "an unknown scope is rejected",
			held:    []Permission{perm("users", ActionRead, "", ScopeAll, EffectAllow)},
			grant:   perm("users", ActionRead, "", Scope("universe"), EffectAllow),
			wantErr: true,
			reason:  "invalid scope",
		},
		{
			name:    "a permission on another resource does not authorize the grant",
			held:    []Permission{perm("roles", ActionCreate, "", ScopeAll, EffectAllow)},
			grant:   perm("users", ActionCreate, "", ScopeTenant, EffectAllow),
			wantErr: true,
			reason:  "you do not hold this permission",
		},
		{
			name: "the widest held grant is what counts",
			held: []Permission{
				perm("users", ActionRead, "", ScopeOwn, EffectAllow),
				perm("users", ActionRead, "", ScopeTenant, EffectAllow),
			},
			grant: perm("users", ActionRead, "", ScopeOrganization, EffectAllow),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			set := NewPermissionSet(tc.held)
			err := set.CanDelegate(tc.grant)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("CanDelegate(%s) = nil, want rejection", tc.grant.Code())
				}
				var de *DelegationError
				if !asDelegationError(err, &de) {
					t.Fatalf("error type = %T, want *DelegationError", err)
				}
				if tc.reason != "" && !contains(de.Reason, tc.reason) {
					t.Errorf("reason = %q, want it to mention %q", de.Reason, tc.reason)
				}
				return
			}
			if err != nil {
				t.Fatalf("CanDelegate(%s) = %v, want success", tc.grant.Code(), err)
			}
		})
	}
}

// A privilege escalation must be impossible even when the actor holds a large
// bundle: the batch check rejects if ANY member is not delegable.
func TestCanDelegateAllRejectsWholeBundleOnOneViolation(t *testing.T) {
	actor := NewPermissionSet([]Permission{
		perm("users", ActionCreate, "", ScopeTenant, EffectAllow),
		perm("users", ActionRead, "", ScopeTenant, EffectAllow),
	})
	bundle := []Permission{
		perm("users", ActionCreate, "", ScopeTenant, EffectAllow), // ok
		perm("users", ActionRead, "", ScopeTenant, EffectAllow),   // ok
		perm("users", ActionDelete, "", ScopeTenant, EffectAllow), // NOT held
	}
	if err := actor.CanDelegateAll(bundle); err == nil {
		t.Fatal("CanDelegateAll accepted a bundle containing a permission the actor lacks")
	}
}

// Granting a deny requires controlling the matching allow, so a principal
// cannot deny a permission it has no authority over.
func TestDelegatingDenyRequiresHoldingTheAllow(t *testing.T) {
	actor := NewPermissionSet([]Permission{perm("users", ActionRead, "", ScopeTenant, EffectAllow)})

	if err := actor.CanDelegateAll([]Permission{perm("users", ActionRead, "", ScopeOwn, EffectDeny)}); err != nil {
		t.Errorf("delegating a deny for a held permission was rejected: %v", err)
	}
	if err := actor.CanDelegateAll([]Permission{perm("users", ActionDelete, "", ScopeOwn, EffectDeny)}); err == nil {
		t.Error("delegating a deny for a permission the actor does not hold was accepted")
	}
}

// ─── Permission evaluation ──────────────────────────────────────────────────

func TestPermissionSetAllows(t *testing.T) {
	selfRel := TargetRelation{IsSelf: true, SameTenant: true}
	tenantRel := TargetRelation{SameTenant: true}
	foreignRel := TargetRelation{}

	tests := []struct {
		name string
		held []Permission
		rel  TargetRelation
		want bool
	}{
		{"deny by default with no permissions", nil, tenantRel, false},
		{"tenant scope reaches same-tenant target", []Permission{perm("users", ActionRead, "", ScopeTenant, EffectAllow)}, tenantRel, true},
		{"tenant scope does not reach a foreign target", []Permission{perm("users", ActionRead, "", ScopeTenant, EffectAllow)}, foreignRel, false},
		{"own scope reaches self", []Permission{perm("users", ActionRead, "", ScopeOwn, EffectAllow)}, selfRel, true},
		{"own scope does not reach another user", []Permission{perm("users", ActionRead, "", ScopeOwn, EffectAllow)}, tenantRel, false},
		{"all scope reaches anything", []Permission{perm("users", ActionRead, "", ScopeAll, EffectAllow)}, foreignRel, true},
		{
			name: "deny overrides allow at the same scope",
			held: []Permission{
				perm("users", ActionRead, "", ScopeTenant, EffectAllow),
				perm("users", ActionRead, "", ScopeTenant, EffectDeny),
			},
			rel:  tenantRel,
			want: false,
		},
		{
			name: "a narrower deny still overrides a wider allow when it reaches the target",
			held: []Permission{
				perm("users", ActionRead, "", ScopeAll, EffectAllow),
				perm("users", ActionRead, "", ScopeOwn, EffectDeny),
			},
			rel:  selfRel,
			want: false,
		},
		{
			name: "a deny that does not reach the target leaves the allow intact",
			held: []Permission{
				perm("users", ActionRead, "", ScopeTenant, EffectAllow),
				perm("users", ActionRead, "", ScopeOwn, EffectDeny),
			},
			rel:  tenantRel, // not self, so the own-scoped deny does not apply
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			set := NewPermissionSet(tc.held)
			if got := set.Allows("user-management", "users", ActionRead, tc.rel); got != tc.want {
				t.Errorf("Allows() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAllowedFieldsExcludesUngrantedAndDeniedFields(t *testing.T) {
	rel := TargetRelation{SameTenant: true}
	set := NewPermissionSet([]Permission{
		perm("users", ActionRead, "", ScopeTenant, EffectAllow),
		perm("users", ActionRead, "email", ScopeTenant, EffectAllow),
		perm("users", ActionRead, "first_name", ScopeTenant, EffectAllow),
		perm("users", ActionRead, "internal_notes", ScopeTenant, EffectAllow),
		perm("users", ActionRead, "internal_notes", ScopeTenant, EffectDeny),
		// A field of a different action must not leak into read's field list.
		perm("users", ActionSearch, "phone", ScopeTenant, EffectAllow),
	})

	got := set.AllowedFields("user-management", "users", ActionRead, rel)
	want := []string{"email", "first_name"}
	if len(got) != len(want) {
		t.Fatalf("AllowedFields() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AllowedFields() = %v, want %v", got, want)
		}
	}
	if set.AllowsField("user-management", "users", ActionRead, "phone", rel) {
		t.Error("AllowsField(phone) = true although no read.phone grant exists")
	}
	if set.AllowsField("user-management", "users", ActionRead, "internal_notes", rel) {
		t.Error("AllowsField(internal_notes) = true although it is explicitly denied")
	}
}

func TestScopeWiderOrEqual(t *testing.T) {
	tests := []struct {
		a, b Scope
		want bool
	}{
		{ScopeAll, ScopeTenant, true},
		{ScopeTenant, ScopeAll, false},
		{ScopeTenant, ScopeTenant, true},
		{ScopeTenant, ScopeOwn, true},
		{ScopeOwn, ScopeTenant, false},
		{ScopeOrganization, ScopeTeam, true},
		{ScopeTeam, ScopeOrganization, false},
	}
	for _, tc := range tests {
		if got := ScopeWiderOrEqual(tc.a, tc.b); got != tc.want {
			t.Errorf("ScopeWiderOrEqual(%s, %s) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestPermissionCode(t *testing.T) {
	if got := perm("users", ActionRead, "", ScopeTenant, EffectAllow).Code(); got != "user-management.users.read" {
		t.Errorf("Code() = %q", got)
	}
	if got := perm("users", ActionRead, "email", ScopeTenant, EffectAllow).Code(); got != "user-management.users.read.email" {
		t.Errorf("Code() = %q", got)
	}
}

func TestParsePermissionCode(t *testing.T) {
	service, resource, action, field, err := ParsePermissionCode("user-management.users.read.email")
	if err != nil {
		t.Fatalf("ParsePermissionCode: %v", err)
	}
	if service != "user-management" || resource != "users" || action != ActionRead || field != "email" {
		t.Errorf("got %s/%s/%s/%s", service, resource, action, field)
	}
	if _, _, _, _, err := ParsePermissionCode("bad"); err == nil {
		t.Error("ParsePermissionCode(\"bad\") = nil error, want failure")
	}
}

// ─── helpers ────────────────────────────────────────────────────────────────

func asDelegationError(err error, target **DelegationError) bool {
	de, ok := err.(*DelegationError)
	if ok {
		*target = de
	}
	return ok
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
