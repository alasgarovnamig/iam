package app

import (
	"context"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
	"github.com/sr-alasgarov/wallet-gateway/internal/events"
	"github.com/sr-alasgarov/wallet-gateway/internal/validation"
)

// PermissionService serves the permission catalog and direct user grants.
type PermissionService struct {
	deps *Deps
	az   *Authorizer
}

func NewPermissionService(d *Deps, az *Authorizer) *PermissionService {
	return &PermissionService{deps: d, az: az}
}

// Catalog returns permission definitions. Tenant principals only see
// tenant-grantable (non-platform-only) entries.
func (s *PermissionService) Catalog(ctx context.Context, p domain.Principal) ([]*domain.PermissionDefinition, error) {
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "permissions", Action: domain.ActionList, Method: "GET", Path: "/api/v1/permissions",
		TargetTenantID: p.TenantID,
	}); err != nil {
		return nil, err
	}
	var defs []*domain.PermissionDefinition
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		defs, err = s.deps.Permissions.ListDefinitions(txCtx)
		return err
	})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if p.IsPlatform() {
		return defs, nil
	}
	filtered := defs[:0]
	for _, d := range defs {
		if !d.IsPlatformOnly {
			filtered = append(filtered, d)
		}
	}
	return filtered, nil
}

type DirectGrantCommand struct {
	PermissionID string
	Scope        string
	Effect       string
	Conditions   map[string]string
}

// GrantToUser gives a user a direct permission, enforcing the delegation
// subset rule and tenant boundaries.
func (s *PermissionService) GrantToUser(ctx context.Context, p domain.Principal, userID string, cmd DirectGrantCommand) (*domain.UserPermission, error) {
	if err := validation.UUID(userID); err != nil {
		return nil, apperr.BadRequest("Invalid user id.")
	}
	if err := validation.UUID(cmd.PermissionID); err != nil {
		return nil, apperr.BadRequest("Invalid permission id.")
	}
	var user *domain.User
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		user, err = s.deps.Users.GetByID(txCtx, userID)
		return err
	})
	if err != nil {
		return nil, apperr.NotFound("User")
	}
	dec, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "permissions", Action: domain.ActionAssign, Method: "POST",
		Path: "/api/v1/users/" + userID + "/permissions", TargetTenantID: user.TenantID, TargetUserID: userID,
	})
	if err != nil {
		return nil, err
	}
	var up *domain.UserPermission
	attempted := map[string]any{"permission_id": cmd.PermissionID, "scope": cmd.Scope, "effect": cmd.Effect}
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		def, err := s.deps.Permissions.GetDefinition(txCtx, cmd.PermissionID)
		if err != nil {
			return apperr.NotFound("Permission definition")
		}
		grant, err := buildGrant(def, cmd.Scope, cmd.Effect, cmd.Conditions)
		if err != nil {
			return err
		}
		attempted["permission"] = grant.Code()
		if !user.IsPlatform() && def.IsPlatformOnly {
			return apperr.DelegationDenied("Platform-only permissions cannot be granted to tenant users.")
		}
		if !user.IsPlatform() && grant.Scope == domain.ScopeAll {
			return apperr.DelegationDenied("Tenant users cannot receive scope=all.")
		}
		if err := dec.PermSet.CanDelegateAll([]domain.Permission{grant}); err != nil {
			return apperr.DelegationDenied(err.Error())
		}
		up = &domain.UserPermission{
			ID: newID(), UserID: userID, TenantID: user.TenantID, Permission: grant,
			GrantedBy: p.ID, CreatedAt: s.deps.now(),
		}
		if err := s.deps.Permissions.GrantToUser(txCtx, up); err != nil {
			return err
		}
		if err := s.deps.recordAudit(txCtx, p, "user.permission_assigned", "users", userID, user.TenantID,
			nil, map[string]any{"permission": grant.Code(), "scope": string(grant.Scope), "effect": string(grant.Effect)},
			nil, domain.AuditSuccess, ""); err != nil {
			return err
		}
		return s.deps.emitEvent(txCtx, p, events.UserPermissionAssignedV1, "user", userID, user.TenantID,
			map[string]any{"permission": grant.Code(), "scope": string(grant.Scope), "effect": string(grant.Effect)})
	})
	if err != nil {
		s.deps.auditDelegationFailure(ctx, p, err, "user.permission_assign", "users", userID, user.TenantID, attempted)
		return nil, apperr.From(err)
	}
	s.az.InvalidatePermissions(ctx, user.TenantID, userID)
	return up, nil
}

func (s *PermissionService) RevokeFromUser(ctx context.Context, p domain.Principal, userID, userPermissionID string) error {
	if err := validation.UUID(userID); err != nil {
		return apperr.BadRequest("Invalid user id.")
	}
	var user *domain.User
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		user, err = s.deps.Users.GetByID(txCtx, userID)
		return err
	})
	if err != nil {
		return apperr.NotFound("User")
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "permissions", Action: domain.ActionRevoke, Method: "DELETE",
		Path:           "/api/v1/users/" + userID + "/permissions/" + userPermissionID,
		TargetTenantID: user.TenantID, TargetUserID: userID,
	}); err != nil {
		return err
	}
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		if err := s.deps.Permissions.RevokeFromUser(txCtx, userID, userPermissionID); err != nil {
			return err
		}
		if err := s.deps.recordAudit(txCtx, p, "user.permission_revoked", "users", userID, user.TenantID,
			nil, map[string]any{"user_permission_id": userPermissionID}, nil, domain.AuditSuccess, ""); err != nil {
			return err
		}
		return s.deps.emitEvent(txCtx, p, events.UserPermissionRevokedV1, "user", userID, user.TenantID,
			map[string]any{"user_permission_id": userPermissionID})
	})
	if err != nil {
		return apperr.From(err)
	}
	s.az.InvalidatePermissions(ctx, user.TenantID, userID)
	return nil
}
