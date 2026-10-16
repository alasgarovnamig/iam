package app

import (
	"context"
	"strings"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
	"github.com/sr-alasgarov/wallet-gateway/internal/events"
	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
	"github.com/sr-alasgarov/wallet-gateway/internal/validation"
)

// RoleService manages custom roles, role permissions (with the delegation
// subset rule) and role assignment.
type RoleService struct {
	deps *Deps
	az   *Authorizer
}

func NewRoleService(d *Deps, az *Authorizer) *RoleService { return &RoleService{deps: d, az: az} }

type CreateRoleCommand struct {
	Name        string
	Description string
	TenantID    string // platform callers may create platform roles ("")
}

func (s *RoleService) Create(ctx context.Context, p domain.Principal, cmd CreateRoleCommand) (*domain.Role, error) {
	targetTenant := cmd.TenantID
	roleType := domain.RoleTypeTenant
	if p.IsPlatform() {
		if targetTenant == "" {
			roleType = domain.RoleTypePlatform
		}
	} else {
		targetTenant = p.TenantID
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "roles", Action: domain.ActionCreate, Method: "POST", Path: "/api/v1/roles",
		TargetTenantID: targetTenant,
	}); err != nil {
		return nil, err
	}
	cmd.Name = strings.TrimSpace(cmd.Name)
	if err := validation.Required("name", cmd.Name); err != nil {
		return nil, apperr.Validation(err.Error())
	}
	if err := validation.MaxLen("name", cmd.Name, 100); err != nil {
		return nil, apperr.Validation(err.Error())
	}
	now := s.deps.now()
	role := &domain.Role{
		ID:          newID(),
		TenantID:    targetTenant,
		Name:        cmd.Name,
		Description: strings.TrimSpace(cmd.Description),
		Type:        roleType,
		IsSystem:    false,
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   p.ID,
		UpdatedBy:   p.ID,
		Version:     1,
	}
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		if err := s.deps.Roles.Create(txCtx, role); err != nil {
			return err
		}
		if err := s.deps.recordAudit(txCtx, p, "role.created", "roles", role.ID, targetTenant,
			nil, map[string]any{"name": role.Name, "type": string(role.Type)},
			[]string{"name", "description"}, domain.AuditSuccess, ""); err != nil {
			return err
		}
		return s.deps.emitEvent(txCtx, p, events.RoleCreatedV1, "role", role.ID, targetTenant,
			map[string]any{"name": role.Name, "type": string(role.Type)})
	})
	if err != nil {
		return nil, apperr.From(err)
	}
	return role, nil
}

func (s *RoleService) Get(ctx context.Context, p domain.Principal, id string) (*domain.Role, []*domain.RolePermission, error) {
	if err := validation.UUID(id); err != nil {
		return nil, nil, apperr.BadRequest("Invalid role id.")
	}
	var role *domain.Role
	var perms []*domain.RolePermission
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		role, err = s.deps.Roles.GetByID(txCtx, id)
		if err != nil {
			return err
		}
		perms, err = s.deps.Roles.ListPermissions(txCtx, id)
		return err
	})
	if err != nil {
		return nil, nil, apperr.NotFound("Role")
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "roles", Action: domain.ActionRead, Method: "GET", Path: "/api/v1/roles/" + id,
		TargetTenantID: role.TenantID,
	}); err != nil {
		return nil, nil, err
	}
	return role, perms, nil
}

func (s *RoleService) List(ctx context.Context, p domain.Principal, page ports.ListPage) ([]*domain.Role, int, error) {
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "roles", Action: domain.ActionList, Method: "GET", Path: "/api/v1/roles",
		TargetTenantID: p.TenantID,
	}); err != nil {
		return nil, 0, err
	}
	sanitizeListPage(&page, map[string]bool{"name": true, "created_at": true}, map[string]bool{"role_type": true, "status": true})
	var roles []*domain.Role
	var total int
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		roles, total, err = s.deps.Roles.List(txCtx, p.TenantID, true, page)
		return err
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	return roles, total, nil
}

type UpdateRoleCommand struct {
	Name        *string
	Description *string
	Status      *string
	Version     int64
}

func (s *RoleService) Update(ctx context.Context, p domain.Principal, id string, cmd UpdateRoleCommand) (*domain.Role, error) {
	if err := validation.UUID(id); err != nil {
		return nil, apperr.BadRequest("Invalid role id.")
	}
	var role *domain.Role
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		role, err = s.deps.Roles.GetByID(txCtx, id)
		return err
	})
	if err != nil {
		return nil, apperr.NotFound("Role")
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "roles", Action: domain.ActionUpdate, Method: "PATCH", Path: "/api/v1/roles/" + id,
		TargetTenantID: role.TenantID,
	}); err != nil {
		return nil, err
	}
	if err := role.EnsureMutable(); err != nil {
		return nil, err
	}
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		role, err = s.deps.Roles.GetByID(txCtx, id)
		if err != nil {
			return apperr.NotFound("Role")
		}
		if cmd.Version != 0 && cmd.Version != role.Version {
			return apperr.VersionConflict()
		}
		before := map[string]any{"name": role.Name, "description": role.Description, "status": role.Status}
		var changed []string
		if cmd.Name != nil && *cmd.Name != role.Name {
			role.Name = strings.TrimSpace(*cmd.Name)
			changed = append(changed, "name")
		}
		if cmd.Description != nil && *cmd.Description != role.Description {
			role.Description = strings.TrimSpace(*cmd.Description)
			changed = append(changed, "description")
		}
		if cmd.Status != nil && *cmd.Status != role.Status {
			if err := validation.OneOf("status", *cmd.Status, "active", "disabled"); err != nil {
				return apperr.Validation(err.Error())
			}
			role.Status = *cmd.Status
			changed = append(changed, "status")
		}
		if len(changed) == 0 {
			return nil
		}
		role.UpdatedBy = p.ID
		if err := s.deps.Roles.Update(txCtx, role); err != nil {
			return err
		}
		after := map[string]any{"name": role.Name, "description": role.Description, "status": role.Status}
		if err := s.deps.recordAudit(txCtx, p, "role.updated", "roles", role.ID, role.TenantID,
			before, after, changed, domain.AuditSuccess, ""); err != nil {
			return err
		}
		return s.deps.emitEvent(txCtx, p, events.RoleUpdatedV1, "role", role.ID, role.TenantID, after)
	})
	if err != nil {
		return nil, apperr.From(err)
	}
	s.az.InvalidateTenantPermissions(ctx, role.TenantID)
	return role, nil
}

func (s *RoleService) Delete(ctx context.Context, p domain.Principal, id string) error {
	if err := validation.UUID(id); err != nil {
		return apperr.BadRequest("Invalid role id.")
	}
	var role *domain.Role
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		role, err = s.deps.Roles.GetByID(txCtx, id)
		return err
	})
	if err != nil {
		return apperr.NotFound("Role")
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "roles", Action: domain.ActionDelete, Method: "DELETE", Path: "/api/v1/roles/" + id,
		TargetTenantID: role.TenantID,
	}); err != nil {
		return err
	}
	if err := role.EnsureMutable(); err != nil {
		return err
	}
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		inUse, err := s.deps.Roles.CountUsersWithRole(txCtx, id)
		if err != nil {
			return apperr.Internal(err)
		}
		if inUse > 0 {
			return apperr.Conflict("Role is assigned to users; revoke assignments first.")
		}
		if err := s.deps.Roles.SoftDelete(txCtx, id, p.ID); err != nil {
			return err
		}
		return s.deps.recordAudit(txCtx, p, "role.deleted", "roles", id, role.TenantID,
			nil, nil, nil, domain.AuditSuccess, "")
	})
	if err != nil {
		return apperr.From(err)
	}
	s.az.InvalidateTenantPermissions(ctx, role.TenantID)
	return nil
}

type GrantRolePermissionCommand struct {
	PermissionID string
	Scope        string
	Effect       string
	Conditions   map[string]string
}

// AddPermission attaches a permission to a role, enforcing:
//   - actor holds permissions.assign,
//   - the delegation subset rule (actor must hold an equal-or-wider grant),
//   - platform-only permissions never reach tenant roles,
//   - system roles are immutable.
func (s *RoleService) AddPermission(ctx context.Context, p domain.Principal, roleID string, cmd GrantRolePermissionCommand) (*domain.RolePermission, error) {
	if err := validation.UUID(roleID); err != nil {
		return nil, apperr.BadRequest("Invalid role id.")
	}
	if err := validation.UUID(cmd.PermissionID); err != nil {
		return nil, apperr.BadRequest("Invalid permission id.")
	}
	var role *domain.Role
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		role, err = s.deps.Roles.GetByID(txCtx, roleID)
		return err
	})
	if err != nil {
		return nil, apperr.NotFound("Role")
	}
	dec, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "permissions", Action: domain.ActionAssign, Method: "POST",
		Path: "/api/v1/roles/" + roleID + "/permissions", TargetTenantID: role.TenantID,
	})
	if err != nil {
		return nil, err
	}
	if err := role.EnsureMutable(); err != nil {
		return nil, err
	}

	var rp *domain.RolePermission
	// attempted describes the grant for the audit trail if we reject it.
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
		// Tenant roles may never carry platform-only permissions.
		if role.Type == domain.RoleTypeTenant && def.IsPlatformOnly {
			return apperr.DelegationDenied("Platform-only permissions cannot be assigned to tenant roles.")
		}
		if role.Type == domain.RoleTypeTenant && grant.Scope == domain.ScopeAll {
			return apperr.DelegationDenied("Tenant roles cannot have scope=all.")
		}
		// THE core rule: the actor may only delegate a subset of what it holds.
		if err := dec.PermSet.CanDelegateAll([]domain.Permission{grant}); err != nil {
			return apperr.DelegationDenied(err.Error())
		}
		rp = &domain.RolePermission{
			ID: newID(), RoleID: roleID, TenantID: role.TenantID, Permission: grant,
			CreatedBy: p.ID, CreatedAt: s.deps.now(),
		}
		if err := s.deps.Roles.AddPermission(txCtx, rp); err != nil {
			return err
		}
		if err := s.deps.recordAudit(txCtx, p, "role.permission_assigned", "roles", roleID, role.TenantID,
			nil, map[string]any{"permission": grant.Code(), "scope": string(grant.Scope), "effect": string(grant.Effect)},
			nil, domain.AuditSuccess, ""); err != nil {
			return err
		}
		return s.deps.emitEvent(txCtx, p, events.RolePermissionAssignedV1, "role", roleID, role.TenantID,
			map[string]any{"permission": grant.Code(), "scope": string(grant.Scope), "effect": string(grant.Effect)})
	})
	if err != nil {
		// The rejection rolled the transaction back, so the attempt is audited
		// separately — privilege-escalation attempts must never be lost.
		s.deps.auditDelegationFailure(ctx, p, err, "role.permission_assign", "roles", roleID, role.TenantID, attempted)
		return nil, apperr.From(err)
	}
	s.az.InvalidateTenantPermissions(ctx, role.TenantID)
	return rp, nil
}

func (s *RoleService) RemovePermission(ctx context.Context, p domain.Principal, roleID, rolePermissionID string) error {
	if err := validation.UUID(roleID); err != nil {
		return apperr.BadRequest("Invalid role id.")
	}
	var role *domain.Role
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		role, err = s.deps.Roles.GetByID(txCtx, roleID)
		return err
	})
	if err != nil {
		return apperr.NotFound("Role")
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "permissions", Action: domain.ActionRevoke, Method: "DELETE",
		Path: "/api/v1/roles/" + roleID + "/permissions/" + rolePermissionID, TargetTenantID: role.TenantID,
	}); err != nil {
		return err
	}
	if err := role.EnsureMutable(); err != nil {
		return err
	}
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		if err := s.deps.Roles.RemovePermission(txCtx, roleID, rolePermissionID); err != nil {
			return err
		}
		if err := s.deps.recordAudit(txCtx, p, "role.permission_revoked", "roles", roleID, role.TenantID,
			nil, nil, nil, domain.AuditSuccess, ""); err != nil {
			return err
		}
		return s.deps.emitEvent(txCtx, p, events.RolePermissionRevokedV1, "role", roleID, role.TenantID,
			map[string]any{"role_permission_id": rolePermissionID})
	})
	if err != nil {
		return apperr.From(err)
	}
	s.az.InvalidateTenantPermissions(ctx, role.TenantID)
	return nil
}

// AssignToUser gives a role to a user. Because a role is a bundle of
// permissions, the delegation rule applies to the whole bundle.
func (s *RoleService) AssignToUser(ctx context.Context, p domain.Principal, userID, roleID string) error {
	if err := validation.UUID(userID); err != nil {
		return apperr.BadRequest("Invalid user id.")
	}
	if err := validation.UUID(roleID); err != nil {
		return apperr.BadRequest("Invalid role id.")
	}
	var user *domain.User
	var role *domain.Role
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		user, err = s.deps.Users.GetByID(txCtx, userID)
		if err != nil {
			return apperr.NotFound("User")
		}
		role, err = s.deps.Roles.GetByID(txCtx, roleID)
		if err != nil {
			return apperr.NotFound("Role")
		}
		return nil
	})
	if err != nil {
		return apperr.From(err)
	}
	dec, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "roles", Action: domain.ActionAssign, Method: "POST",
		Path: "/api/v1/users/" + userID + "/roles", TargetTenantID: user.TenantID, TargetUserID: userID,
	})
	if err != nil {
		return err
	}
	if err := validateRoleAssignable(p, user, role); err != nil {
		return err
	}
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		rolePerms, err := s.deps.Roles.ListPermissions(txCtx, roleID)
		if err != nil {
			return apperr.Internal(err)
		}
		grants := make([]domain.Permission, 0, len(rolePerms))
		for _, rp := range rolePerms {
			grants = append(grants, rp.Permission)
		}
		// Assigning a role delegates every permission it bundles.
		if err := dec.PermSet.CanDelegateAll(grants); err != nil {
			return apperr.DelegationDenied(err.Error())
		}
		if err := s.deps.Roles.AssignToUser(txCtx, &domain.UserRole{
			ID: newID(), UserID: userID, RoleID: roleID, TenantID: user.TenantID,
			AssignedBy: p.ID, CreatedAt: s.deps.now(),
		}); err != nil {
			return err
		}
		if err := s.deps.recordAudit(txCtx, p, "user.role_assigned", "users", userID, user.TenantID,
			nil, map[string]any{"role_id": roleID, "role_name": role.Name}, nil, domain.AuditSuccess, ""); err != nil {
			return err
		}
		return s.deps.emitEvent(txCtx, p, events.UserRoleAssignedV1, "user", userID, user.TenantID,
			map[string]any{"role_id": roleID, "role_name": role.Name})
	})
	if err != nil {
		s.deps.auditDelegationFailure(ctx, p, err, "user.role_assign", "users", userID, user.TenantID,
			map[string]any{"role_id": roleID, "role_name": role.Name})
		return apperr.From(err)
	}
	s.az.InvalidatePermissions(ctx, user.TenantID, userID)
	return nil
}

func (s *RoleService) RevokeFromUser(ctx context.Context, p domain.Principal, userID, roleID string) error {
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
		Resource: "roles", Action: domain.ActionRevoke, Method: "DELETE",
		Path: "/api/v1/users/" + userID + "/roles/" + roleID, TargetTenantID: user.TenantID, TargetUserID: userID,
	}); err != nil {
		return err
	}
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		if err := s.deps.Roles.RevokeFromUser(txCtx, userID, roleID); err != nil {
			return err
		}
		if err := s.deps.recordAudit(txCtx, p, "user.role_revoked", "users", userID, user.TenantID,
			nil, map[string]any{"role_id": roleID}, nil, domain.AuditSuccess, ""); err != nil {
			return err
		}
		return s.deps.emitEvent(txCtx, p, events.UserRoleRevokedV1, "user", userID, user.TenantID,
			map[string]any{"role_id": roleID})
	})
	if err != nil {
		return apperr.From(err)
	}
	s.az.InvalidatePermissions(ctx, user.TenantID, userID)
	return nil
}

// buildGrant converts a catalog definition + scope/effect into a Permission.
func buildGrant(def *domain.PermissionDefinition, scope, effect string, conds map[string]string) (domain.Permission, error) {
	if effect == "" {
		effect = "allow"
	}
	if effect != "allow" && effect != "deny" {
		return domain.Permission{}, apperr.Validation("effect must be allow or deny")
	}
	sc := domain.Scope(scope)
	if scope == "" {
		sc = domain.ScopeOwn
	}
	if !domain.ValidScope(sc) {
		return domain.Permission{}, apperr.Validation("invalid scope: " + scope)
	}
	return domain.Permission{
		DefinitionID: def.ID,
		Service:      def.Service,
		Resource:     def.Resource,
		Action:       def.Action,
		Field:        def.Field,
		Scope:        sc,
		Effect:       domain.Effect(effect),
		Conditions:   conds,
	}, nil
}
