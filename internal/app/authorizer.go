package app

import (
	"context"
	"fmt"
	"time"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
)

const (
	ServiceName        = "user-management"
	permissionCacheTTL = 2 * time.Minute
)

// AuthzRequest describes what the caller wants to do.
type AuthzRequest struct {
	Resource       string
	Action         domain.Action
	Method         string
	Path           string
	TargetTenantID string // tenant of the target resource ("" = platform scope)
	TargetUserID   string // for own-scope / self checks
	TargetOwnerID  string // resource owner (e.g. session's user)
}

// Decision is the combined authorization outcome used by services/handlers.
type Decision struct {
	Allow         bool
	Reason        string
	AllowedFields []string
	PermSet       domain.PermissionSet
	Relation      domain.TargetRelation
}

// Authorizer combines the three authorization layers:
//  1. relationship checks (OpenFGA),
//  2. request policy (OPA),
//  3. resource/field permission evaluation (domain PermissionSet).
//
// Every failure path is fail-closed.
type Authorizer struct {
	deps *Deps
}

func NewAuthorizer(d *Deps) *Authorizer { return &Authorizer{deps: d} }

// EffectivePermissions loads (cache-first) the principal's permission set.
func (a *Authorizer) EffectivePermissions(ctx context.Context, p domain.Principal) (domain.PermissionSet, error) {
	if perms, ok := a.deps.Cache.Get(ctx, p.TenantID, p.ID); ok {
		return domain.NewPermissionSet(perms), nil
	}
	var perms []domain.Permission
	err := a.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		if p.IsAPIClient() {
			perms, err = a.deps.Permissions.EffectiveForAPIClient(txCtx, p.ID)
		} else {
			perms, err = a.deps.Permissions.EffectiveForUser(txCtx, p.ID)
		}
		return err
	})
	if err != nil {
		// Fail closed: no permissions on error.
		return domain.PermissionSet{}, apperr.Internal(fmt.Errorf("load effective permissions: %w", err))
	}
	a.deps.Cache.Set(ctx, p.TenantID, p.ID, perms, permissionCacheTTL)
	return domain.NewPermissionSet(perms), nil
}

// relation computes the principal→target relation used for scope evaluation.
func (a *Authorizer) relation(p domain.Principal, req AuthzRequest) domain.TargetRelation {
	rel := domain.TargetRelation{Platform: p.IsPlatform()}
	if req.TargetUserID != "" && req.TargetUserID == p.ID {
		rel.IsSelf = true
	}
	if req.TargetOwnerID != "" && req.TargetOwnerID == p.ID {
		rel.IsSelf = true
	}
	if p.TenantID != "" && p.TenantID == req.TargetTenantID {
		rel.SameTenant = true
	}
	if p.IsPlatform() {
		// Platform principals operate across tenants; their grants use
		// scope=all which ignores the relation anyway.
		rel.SameTenant = req.TargetTenantID != "" || rel.SameTenant
	}
	return rel
}

// checkRelationship consults OpenFGA. Errors deny (fail closed).
func (a *Authorizer) checkRelationship(ctx context.Context, p domain.Principal, req AuthzRequest) (bool, string, error) {
	// API clients act on behalf of their tenant; membership is implicit in
	// the client credential, but the tenant boundary still applies.
	if p.IsAPIClient() {
		if req.TargetTenantID != "" && req.TargetTenantID != p.TenantID {
			return false, "cross_tenant_denied", nil
		}
		return true, "api_client_tenant_scope", nil
	}
	if p.IsPlatform() {
		ok, err := a.deps.FGA.Check(ctx, "user:"+p.ID, "member", "platform:platform")
		if err != nil {
			return false, "relationship_check_unavailable", err
		}
		return ok, "platform_membership", nil
	}
	// Tenant principals must be members of their tenant and may never cross it.
	if req.TargetTenantID != "" && req.TargetTenantID != p.TenantID {
		return false, "cross_tenant_denied", nil
	}
	ok, err := a.deps.FGA.Check(ctx, "user:"+p.ID, "member", "tenant:"+p.TenantID)
	if err != nil {
		return false, "relationship_check_unavailable", err
	}
	return ok, "tenant_membership", nil
}

// Authorize runs the full decision pipeline.
func (a *Authorizer) Authorize(ctx context.Context, p domain.Principal, req AuthzRequest) (*Decision, error) {
	permSet, err := a.EffectivePermissions(ctx, p)
	if err != nil {
		return nil, err
	}
	rel := a.relation(p, req)

	relOK, relReason, relErr := a.checkRelationship(ctx, p, req)
	if relErr != nil {
		a.deps.Logger.Warn("relationship check failed, denying", "error", relErr)
		a.deps.auditDenied(ctx, p, string(req.Action), req.Resource, "", req.TargetTenantID, "relationship_check_unavailable")
		return nil, apperr.Unavailable("Authorization backend is unavailable.")
	}
	if !relOK {
		a.deps.auditDenied(ctx, p, string(req.Action), req.Resource, "", req.TargetTenantID, relReason)
		return &Decision{Allow: false, Reason: relReason, PermSet: permSet, Relation: rel}, nil
	}

	// Build OPA input.
	in := ports.PolicyInput{}
	in.Principal.ID = p.ID
	in.Principal.Type = string(p.Kind)
	in.Principal.TenantID = p.TenantID
	in.Request.Method = req.Method
	in.Request.Path = req.Path
	in.Request.Action = string(req.Action)
	in.Request.Resource = req.Resource
	in.Target.TenantID = req.TargetTenantID
	in.Target.ResourceID = req.TargetUserID
	in.Relationships = map[string]bool{
		"is_self":     rel.IsSelf,
		"same_tenant": rel.SameTenant,
		"platform":    rel.Platform,
		"member":      relOK,
	}
	meta := MetaFrom(ctx)
	in.Context.IP = meta.IP
	in.Context.Time = a.deps.now().Format(time.RFC3339)
	for _, perm := range permSet.All() {
		in.Permissions = append(in.Permissions, ports.PolicyPermission{
			Code: perm.Code(), Scope: string(perm.Scope), Effect: string(perm.Effect), Conds: perm.Conditions,
		})
	}

	dec, err := a.deps.OPA.Decide(ctx, in)
	if err != nil {
		// OPA unreachable => deny (fail closed).
		a.deps.Logger.Warn("OPA decision failed, denying", "error", err)
		a.deps.auditDenied(ctx, p, string(req.Action), req.Resource, "", req.TargetTenantID, "policy_engine_unavailable")
		return nil, apperr.Unavailable("Authorization policy engine is unavailable.")
	}
	if !dec.Allow {
		a.deps.auditDenied(ctx, p, string(req.Action), req.Resource, "", req.TargetTenantID, dec.Reason)
		return &Decision{Allow: false, Reason: dec.Reason, PermSet: permSet, Relation: rel}, nil
	}

	// Defence in depth: re-verify the action-level permission in the domain
	// engine (OPA and this must agree; a mismatch denies).
	if !permSet.Allows(ServiceName, req.Resource, req.Action, rel) {
		a.deps.auditDenied(ctx, p, string(req.Action), req.Resource, "", req.TargetTenantID, "permission_missing")
		return &Decision{Allow: false, Reason: "permission_missing", PermSet: permSet, Relation: rel}, nil
	}

	allowedFields := permSet.AllowedFields(ServiceName, req.Resource, req.Action, rel)
	return &Decision{
		Allow:         true,
		Reason:        dec.Reason,
		AllowedFields: allowedFields,
		PermSet:       permSet,
		Relation:      rel,
	}, nil
}

// MustAllow is a convenience wrapper returning a typed error on deny.
func (a *Authorizer) MustAllow(ctx context.Context, p domain.Principal, req AuthzRequest) (*Decision, error) {
	dec, err := a.Authorize(ctx, p, req)
	if err != nil {
		return nil, err
	}
	if !dec.Allow {
		return nil, apperr.PermissionDenied("You do not have permission to " + string(req.Action) + " " + req.Resource + ".")
	}
	return dec, nil
}

// InvalidatePermissions drops cached permissions for a principal and its tenant.
func (a *Authorizer) InvalidatePermissions(ctx context.Context, tenantID, principalID string) {
	a.deps.Cache.InvalidatePrincipal(ctx, tenantID, principalID)
}

// InvalidateTenantPermissions drops all cached permissions in a tenant (role
// permission changes affect many principals).
func (a *Authorizer) InvalidateTenantPermissions(ctx context.Context, tenantID string) {
	a.deps.Cache.InvalidateTenant(ctx, tenantID)
}
