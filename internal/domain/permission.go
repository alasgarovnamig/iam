package domain

import (
	"fmt"
	"sort"
	"strings"
)

// Action enumerates permission actions.
type Action string

const (
	ActionCreate Action = "create"
	ActionRead   Action = "read"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
	ActionSearch Action = "search"
	ActionList   Action = "list"
	ActionAssign Action = "assign"
	ActionRevoke Action = "revoke"
	ActionExport Action = "export"
	ActionRotate Action = "rotate"
)

// Scope bounds which resources a permission reaches.
type Scope string

const (
	ScopeOwn          Scope = "own"
	ScopeAssigned     Scope = "assigned"
	ScopeTeam         Scope = "team"
	ScopeOrganization Scope = "organization"
	ScopeDescendants  Scope = "descendants"
	ScopeTenant       Scope = "tenant"
	ScopeAll          Scope = "all"
)

// scopeRank defines the strict widening order used for subset checks.
var scopeRank = map[Scope]int{
	ScopeOwn:          1,
	ScopeAssigned:     1,
	ScopeTeam:         2,
	ScopeOrganization: 3,
	ScopeDescendants:  4,
	ScopeTenant:       5,
	ScopeAll:          6,
}

func ValidScope(s Scope) bool { _, ok := scopeRank[s]; return ok }

// ScopeWiderOrEqual reports whether a is at least as wide as b.
func ScopeWiderOrEqual(a, b Scope) bool { return scopeRank[a] >= scopeRank[b] }

type Effect string

const (
	EffectAllow Effect = "allow"
	EffectDeny  Effect = "deny"
)

// Permission is the canonical permission value object:
// service.resource.action[.field] + scope + effect + conditions.
type Permission struct {
	DefinitionID string // permission_definitions.id
	Service      string
	Resource     string
	Action       Action
	Field        string // "" = action-level
	Scope        Scope
	Effect       Effect
	Conditions   map[string]string
}

// Code renders the canonical dotted code, e.g. user-management.users.read.email.
func (p Permission) Code() string {
	if p.Field == "" {
		return fmt.Sprintf("%s.%s.%s", p.Service, p.Resource, p.Action)
	}
	return fmt.Sprintf("%s.%s.%s.%s", p.Service, p.Resource, p.Action, p.Field)
}

func (p Permission) matchesDefinition(service, resource string, action Action, field string) bool {
	return p.Service == service && p.Resource == resource && p.Action == action && p.Field == field
}

// conditionsSubsetOf reports whether "grant" conditions are equal to or more
// restrictive than "held" conditions: every condition the actor is bound by
// must also bind the grantee (the grant may ADD further restrictions).
func conditionsSubsetOf(grant, held map[string]string) bool {
	for k, v := range held {
		gv, ok := grant[k]
		if !ok || gv != v {
			return false
		}
	}
	return true
}

// TargetRelation captures the relationship between an acting principal and a
// target resource, resolved by the caller (repository / OpenFGA).
type TargetRelation struct {
	IsSelf     bool // target is the principal itself
	SameTenant bool // target belongs to the principal's tenant
	SameOrg    bool // same organization unit subtree
	SameTeam   bool // same immediate organization unit
	Assigned   bool // target explicitly assigned to the principal
	Platform   bool // principal is platform-scoped (tenant_id empty)
}

// ScopeSatisfied reports whether a permission scope reaches the target.
func ScopeSatisfied(s Scope, rel TargetRelation) bool {
	switch s {
	case ScopeAll:
		// "all" is platform-wide; for tenant principals it can never be held
		// (delegation prevents it), but if present it reaches everything.
		return true
	case ScopeTenant:
		return rel.SameTenant || rel.IsSelf
	case ScopeDescendants:
		return rel.SameOrg || rel.IsSelf
	case ScopeOrganization:
		return rel.SameOrg || rel.IsSelf
	case ScopeTeam:
		return rel.SameTeam || rel.IsSelf
	case ScopeAssigned:
		return rel.Assigned || rel.IsSelf
	case ScopeOwn:
		return rel.IsSelf
	default:
		return false
	}
}

// PermissionSet is a principal's effective permission collection.
// Deny always overrides allow.
type PermissionSet struct {
	perms []Permission
}

func NewPermissionSet(perms []Permission) PermissionSet {
	return PermissionSet{perms: perms}
}

func (s PermissionSet) All() []Permission { return s.perms }

func (s PermissionSet) IsEmpty() bool { return len(s.perms) == 0 }

// deniedAt reports whether an explicit deny exists for the definition that
// reaches the given relation. An action-level deny (field "") also denies
// every field of that action.
func (s PermissionSet) deniedAt(service, resource string, action Action, field string, rel TargetRelation) bool {
	for _, p := range s.perms {
		if p.Effect != EffectDeny || p.Service != service || p.Resource != resource || p.Action != action {
			continue
		}
		if p.Field != "" && p.Field != field {
			continue
		}
		if ScopeSatisfied(p.Scope, rel) {
			return true
		}
	}
	return false
}

// Allows checks an action-level permission against a target relation.
func (s PermissionSet) Allows(service, resource string, action Action, rel TargetRelation) bool {
	if s.deniedAt(service, resource, action, "", rel) {
		return false
	}
	for _, p := range s.perms {
		if p.Effect == EffectAllow && p.matchesDefinition(service, resource, action, "") && ScopeSatisfied(p.Scope, rel) {
			return true
		}
	}
	return false
}

// AllowsField checks a field-level permission. Field permissions require an
// explicit field grant; an action-level grant does NOT imply protected fields.
func (s PermissionSet) AllowsField(service, resource string, action Action, field string, rel TargetRelation) bool {
	if s.deniedAt(service, resource, action, field, rel) {
		return false
	}
	for _, p := range s.perms {
		if p.Effect == EffectAllow && p.matchesDefinition(service, resource, action, field) && ScopeSatisfied(p.Scope, rel) {
			return true
		}
	}
	return false
}

// AllowedFields returns the sorted list of explicitly granted (and not
// denied) fields for service/resource/action reaching the target.
func (s PermissionSet) AllowedFields(service, resource string, action Action, rel TargetRelation) []string {
	set := map[string]struct{}{}
	for _, p := range s.perms {
		if p.Effect == EffectAllow && p.Service == service && p.Resource == resource &&
			p.Action == action && p.Field != "" && ScopeSatisfied(p.Scope, rel) {
			set[p.Field] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for f := range set {
		if !s.deniedAt(service, resource, action, f, rel) {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

// MaxScopeFor returns the widest allowed scope the set holds for a definition
// (ok=false when no allow exists or the definition is denied at that scope).
func (s PermissionSet) MaxScopeFor(service, resource string, action Action, field string) (Scope, bool) {
	best := Scope("")
	found := false
	for _, p := range s.perms {
		if p.Effect != EffectAllow || !p.matchesDefinition(service, resource, action, field) {
			continue
		}
		if !found || scopeRank[p.Scope] > scopeRank[best] {
			best, found = p.Scope, true
		}
	}
	return best, found
}

// DelegationError explains why a delegation was rejected.
type DelegationError struct {
	Permission string
	Reason     string
}

func (e *DelegationError) Error() string {
	return fmt.Sprintf("cannot delegate %s: %s", e.Permission, e.Reason)
}

// CanDelegate enforces the core delegation rule: a principal may only grant a
// permission that is an equal-or-narrower subset of a permission it holds:
//   - identical service/resource/action/field (an action-level grant is NOT
//     implied by field-level holdings and vice versa),
//   - equal or narrower scope,
//   - equal or more restrictive conditions,
//   - no explicit deny on the definition,
//   - deny grants additionally require holding the equivalent allow.
func (s PermissionSet) CanDelegate(grant Permission) error {
	code := grant.Code()
	if !ValidScope(grant.Scope) {
		return &DelegationError{Permission: code, Reason: "invalid scope"}
	}
	// An explicit deny anywhere on this definition blocks delegating it.
	for _, p := range s.perms {
		if p.Effect == EffectDeny && p.Service == grant.Service && p.Resource == grant.Resource &&
			p.Action == grant.Action && (p.Field == "" || p.Field == grant.Field) {
			return &DelegationError{Permission: code, Reason: "the permission is explicitly denied for you"}
		}
	}
	var candidates []Permission
	for _, p := range s.perms {
		if p.Effect == EffectAllow && p.matchesDefinition(grant.Service, grant.Resource, grant.Action, grant.Field) {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		return &DelegationError{Permission: code, Reason: "you do not hold this permission"}
	}
	for _, held := range candidates {
		if !ScopeWiderOrEqual(held.Scope, grant.Scope) {
			continue
		}
		if !conditionsSubsetOf(grant.Conditions, held.Conditions) {
			continue
		}
		return nil // subset of at least one held permission
	}
	// Distinguish scope vs condition failure for a useful message.
	for _, held := range candidates {
		if !ScopeWiderOrEqual(held.Scope, grant.Scope) {
			return &DelegationError{Permission: code,
				Reason: fmt.Sprintf("requested scope %q is wider than your scope %q", grant.Scope, held.Scope)}
		}
	}
	return &DelegationError{Permission: code, Reason: "requested conditions are less restrictive than yours"}
}

// CanDelegateAll validates a batch; returns the first violation.
func (s PermissionSet) CanDelegateAll(grants []Permission) error {
	for _, g := range grants {
		toCheck := g
		if g.Effect == EffectDeny {
			// Granting a deny requires controlling the matching allow.
			toCheck.Effect = EffectAllow
		}
		if err := s.CanDelegate(toCheck); err != nil {
			return err
		}
	}
	return nil
}

// ParsePermissionCode parses "service.resource.action[.field]".
func ParsePermissionCode(code string) (service, resource string, action Action, field string, err error) {
	parts := strings.Split(code, ".")
	if len(parts) < 3 || len(parts) > 4 {
		return "", "", "", "", fmt.Errorf("invalid permission code %q", code)
	}
	service, resource = parts[0], parts[1]
	action = Action(parts[2])
	if len(parts) == 4 {
		field = parts[3]
	}
	return service, resource, action, field, nil
}
