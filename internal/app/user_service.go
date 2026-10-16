package app

import (
	"context"
	"strings"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/auth"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
	"github.com/sr-alasgarov/wallet-gateway/internal/events"
	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
	"github.com/sr-alasgarov/wallet-gateway/internal/validation"
)

// UserService implements user CRUD with tenant isolation and field-level
// permission enforcement (read filtering, update/create rejection, search
// field allow-listing).
type UserService struct {
	deps *Deps
	az   *Authorizer
}

func NewUserService(d *Deps, az *Authorizer) *UserService { return &UserService{deps: d, az: az} }

// UserView is a user plus the fields the caller may see; the HTTP layer
// serializes only AllowedFields.
type UserView struct {
	User          *domain.User
	AllowedFields []string
}

type CreateUserCommand struct {
	TenantID  string // empty => platform user (platform admins only)
	Email     string
	Password  string
	FirstName string
	LastName  string
	Phone     string
	RoleIDs   []string
	// ProvidedFields lists JSON keys present in the request body
	// (mass-assignment guard: unknown/forbidden keys reject the request).
	ProvidedFields []string
}

// creatableFields are the JSON keys accepted by user create/update bodies.
var userWritableKeys = map[string]bool{
	"tenant_id": true, "email": true, "password": true, "first_name": true,
	"last_name": true, "phone": true, "role_ids": true, "status": true,
	"internal_notes": true, "version": true,
}

func (s *UserService) Create(ctx context.Context, p domain.Principal, cmd CreateUserCommand) (*UserView, error) {
	// Reject unknown body fields outright (mass assignment protection).
	for _, f := range cmd.ProvidedFields {
		if !userWritableKeys[f] {
			return nil, apperr.BadRequest("Unknown field in request body: " + f)
		}
	}
	targetTenant := cmd.TenantID
	if !p.IsPlatform() {
		// Tenant principals can only ever create users inside their tenant.
		if targetTenant != "" && targetTenant != p.TenantID {
			return nil, apperr.PermissionDenied("You cannot create users in another tenant.")
		}
		targetTenant = p.TenantID
	}
	dec, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "users", Action: domain.ActionCreate, Method: "POST", Path: "/api/v1/users",
		TargetTenantID: targetTenant,
	})
	if err != nil {
		return nil, err
	}
	// Field-level guard: writing protected fields requires update.<field>.
	writable := fieldSet(dec.PermSet.AllowedFields(ServiceName, "users", domain.ActionUpdate, dec.Relation))
	for _, f := range cmd.ProvidedFields {
		if domain.Contains(domain.UserProtectedUpdateFields, f) && !writable[f] {
			return nil, apperr.FieldNotAllowed(f)
		}
	}

	cmd.Email = strings.TrimSpace(strings.ToLower(cmd.Email))
	if err := validation.Email(cmd.Email); err != nil {
		return nil, apperr.Validation(err.Error())
	}
	if err := auth.ValidatePasswordPolicy(cmd.Password); err != nil {
		return nil, apperr.Validation(err.Error())
	}
	userType := domain.UserTypeTenant
	if targetTenant == "" {
		userType = domain.UserTypePlatform
	}
	now := s.deps.now()
	user := &domain.User{
		ID:                 newID(),
		TenantID:           targetTenant,
		Type:               userType,
		Email:              cmd.Email,
		FirstName:          strings.TrimSpace(cmd.FirstName),
		LastName:           strings.TrimSpace(cmd.LastName),
		Phone:              strings.TrimSpace(cmd.Phone),
		Status:             domain.UserActive,
		MustChangePassword: true, // first login forces a change
		SecurityFlags:      map[string]any{},
		CreatedAt:          now,
		UpdatedAt:          now,
		CreatedBy:          p.ID,
		UpdatedBy:          p.ID,
		Version:            1,
	}
	hash, err := s.deps.Hasher.Hash(cmd.Password)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		if targetTenant != "" {
			tenant, err := s.deps.Tenants.GetByID(txCtx, targetTenant)
			if err != nil {
				return apperr.NotFound("Tenant")
			}
			if !tenant.IsOperational() {
				return apperr.Conflict("Tenant is not active.")
			}
		}
		if existing, err := s.deps.Users.GetByEmail(txCtx, targetTenant, cmd.Email); err == nil && existing != nil {
			return apperr.Conflict("A user with this email already exists.")
		}
		if err := s.deps.Users.Create(txCtx, user); err != nil {
			return err
		}
		if err := s.deps.Credentials.Create(txCtx, &domain.Credential{
			UserID: user.ID, TenantID: targetTenant, PasswordHash: hash, PasswordChangedAt: now,
		}); err != nil {
			return err
		}
		// Optional initial roles go through the full delegation check.
		for _, roleID := range cmd.RoleIDs {
			if err := s.assignRoleWithinTx(txCtx, p, dec, user, roleID); err != nil {
				return err
			}
		}
		if err := s.deps.recordAudit(txCtx, p, "user.created", "users", user.ID, targetTenant,
			nil, map[string]any{"email": user.Email, "first_name": user.FirstName, "last_name": user.LastName},
			[]string{"email", "first_name", "last_name"}, domain.AuditSuccess, ""); err != nil {
			return err
		}
		return s.deps.emitEvent(txCtx, p, events.UserCreatedV1, "user", user.ID, targetTenant,
			map[string]any{"email": user.Email, "tenant_id": targetTenant, "type": string(userType)})
	})
	if err != nil {
		return nil, apperr.From(err)
	}
	// Relationship tuples (outside tx; FGA is source of relationship truth,
	// re-derivable from the DB if this write fails).
	tuples := []ports.RelationTuple{}
	if targetTenant != "" {
		tuples = append(tuples, ports.RelationTuple{User: "user:" + user.ID, Relation: "member", Object: "tenant:" + targetTenant})
	} else {
		tuples = append(tuples, ports.RelationTuple{User: "user:" + user.ID, Relation: "member", Object: "platform:platform"})
	}
	if err := s.deps.FGA.Write(ctx, tuples, nil); err != nil {
		s.deps.Logger.Error("failed to write user FGA tuples", "user_id", user.ID, "error", err)
	}
	return &UserView{User: user, AllowedFields: dec.PermSet.AllowedFields(ServiceName, "users", domain.ActionRead, dec.Relation)}, nil
}

// assignRoleWithinTx assigns a role during user creation, enforcing the
// delegation subset rule against the actor's permissions.
func (s *UserService) assignRoleWithinTx(ctx context.Context, p domain.Principal, dec *Decision, user *domain.User, roleID string) error {
	role, err := s.deps.Roles.GetByID(ctx, roleID)
	if err != nil {
		return apperr.NotFound("Role " + roleID)
	}
	if err := validateRoleAssignable(p, user, role); err != nil {
		return err
	}
	rolePerms, err := s.deps.Roles.ListPermissions(ctx, roleID)
	if err != nil {
		return apperr.Internal(err)
	}
	grants := make([]domain.Permission, 0, len(rolePerms))
	for _, rp := range rolePerms {
		grants = append(grants, rp.Permission)
	}
	if err := dec.PermSet.CanDelegateAll(grants); err != nil {
		return apperr.DelegationDenied(err.Error())
	}
	return s.deps.Roles.AssignToUser(ctx, &domain.UserRole{
		ID: newID(), UserID: user.ID, RoleID: roleID, TenantID: user.TenantID,
		AssignedBy: p.ID, CreatedAt: s.deps.now(),
	})
}

// validateRoleAssignable enforces role/tenant/type compatibility.
func validateRoleAssignable(p domain.Principal, user *domain.User, role *domain.Role) error {
	if role.Status != "active" || role.DeletedAt != nil {
		return apperr.Conflict("Role is not active.")
	}
	if user.IsPlatform() {
		if role.Type != domain.RoleTypePlatform {
			return apperr.Validation("Platform users can only receive platform roles.")
		}
	} else {
		if role.Type != domain.RoleTypeTenant {
			return apperr.Validation("Tenant users can only receive tenant roles.")
		}
		if role.TenantID != "" && role.TenantID != user.TenantID {
			return apperr.PermissionDenied("Role belongs to another tenant.")
		}
	}
	if role.Type == domain.RoleTypePlatform && !p.IsPlatform() {
		return apperr.PermissionDenied("Tenant principals cannot assign platform roles.")
	}
	return nil
}

func (s *UserService) Get(ctx context.Context, p domain.Principal, id string) (*UserView, error) {
	if err := validation.UUID(id); err != nil {
		return nil, apperr.BadRequest("Invalid user id.")
	}
	var user *domain.User
	// Load first (under RLS) to learn the target tenant, then authorize.
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		user, err = s.deps.Users.GetByID(txCtx, id)
		return err
	})
	if err != nil {
		return nil, apperr.NotFound("User")
	}
	dec, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "users", Action: domain.ActionRead, Method: "GET", Path: "/api/v1/users/" + id,
		TargetTenantID: user.TenantID, TargetUserID: user.ID,
	})
	if err != nil {
		return nil, err
	}
	return &UserView{User: user, AllowedFields: dec.AllowedFields}, nil
}

type ListUsersQuery struct {
	TenantID string
	Page     ports.ListPage
}

// List enforces list permission plus per-field search permissions.
func (s *UserService) List(ctx context.Context, p domain.Principal, q ListUsersQuery) ([]*UserView, int, error) {
	targetTenant := q.TenantID
	if !p.IsPlatform() {
		targetTenant = p.TenantID
	}
	dec, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "users", Action: domain.ActionList, Method: "GET", Path: "/api/v1/users",
		TargetTenantID: targetTenant,
	})
	if err != nil {
		return nil, 0, err
	}
	// Search/filter fields require users.search.<field>.
	searchable := fieldSet(dec.PermSet.AllowedFields(ServiceName, "users", domain.ActionSearch, dec.Relation))
	canSearch := dec.PermSet.Allows(ServiceName, "users", domain.ActionSearch, dec.Relation)
	for f := range q.Page.Filters {
		if !domain.Contains(domain.UserSearchableFields, f) {
			return nil, 0, apperr.BadRequest("Unknown filter field: " + f)
		}
		if !canSearch || !searchable[f] {
			return nil, 0, apperr.FieldNotAllowed("filter:" + f)
		}
	}
	if q.Page.Search != "" && !canSearch {
		return nil, 0, apperr.PermissionDenied("You do not have search permission on users.")
	}
	sanitizeListPage(&q.Page,
		map[string]bool{"email": true, "first_name": true, "last_name": true, "status": true, "created_at": true},
		fieldSet(domain.UserSearchableFields))

	readFields := dec.PermSet.AllowedFields(ServiceName, "users", domain.ActionRead, dec.Relation)
	var users []*domain.User
	var total int
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		users, total, err = s.deps.Users.List(txCtx, targetTenant, q.Page)
		return err
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	views := make([]*UserView, len(users))
	for i, u := range users {
		views[i] = &UserView{User: u, AllowedFields: readFields}
	}
	return views, total, nil
}

// Search runs a dynamic AND/OR filter tree over users. Two-stage authz: the
// actor must hold users.search, and users.search.<field> for every field the
// filter or sort touches. Result rows are then read-field filtered so a caller
// can filter by a field it may not read (e.g. narrow by status without seeing
// e-mail) but never see a field it lacks read permission for.
func (s *UserService) Search(ctx context.Context, p domain.Principal, tenantID string, cmd SearchCommand) ([]*UserView, int, error) {
	targetTenant := tenantID
	if !p.IsPlatform() {
		targetTenant = p.TenantID
	}
	dec, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "users", Action: domain.ActionSearch, Method: "POST", Path: "/api/v1/users/search",
		TargetTenantID: targetTenant,
	})
	if err != nil {
		return nil, 0, err
	}
	q, err := authorizeSearch(dec, domain.UsersSearchSchema, cmd)
	if err != nil {
		return nil, 0, err
	}
	readFields := dec.PermSet.AllowedFields(ServiceName, "users", domain.ActionRead, dec.Relation)
	var users []*domain.User
	var total int
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		users, total, err = s.deps.Users.Search(txCtx, targetTenant, q)
		return err
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	views := make([]*UserView, len(users))
	for i, u := range users {
		views[i] = &UserView{User: u, AllowedFields: readFields}
	}
	return views, total, nil
}

type UpdateUserCommand struct {
	Email          *string
	FirstName      *string
	LastName       *string
	Phone          *string
	InternalNotes  *string
	Version        int64
	ProvidedFields []string
}

func (s *UserService) Update(ctx context.Context, p domain.Principal, id string, cmd UpdateUserCommand) (*UserView, error) {
	if err := validation.UUID(id); err != nil {
		return nil, apperr.BadRequest("Invalid user id.")
	}
	for _, f := range cmd.ProvidedFields {
		if !userWritableKeys[f] || f == "password" || f == "tenant_id" || f == "role_ids" || f == "status" {
			return nil, apperr.BadRequest("Field cannot be updated through this endpoint: " + f)
		}
	}
	var user *domain.User
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		user, err = s.deps.Users.GetByID(txCtx, id)
		return err
	})
	if err != nil {
		return nil, apperr.NotFound("User")
	}
	dec, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "users", Action: domain.ActionUpdate, Method: "PATCH", Path: "/api/v1/users/" + id,
		TargetTenantID: user.TenantID, TargetUserID: user.ID,
	})
	if err != nil {
		return nil, err
	}
	// Field-level: every provided protected field needs users.update.<field>.
	writable := fieldSet(dec.AllowedFields)
	for _, f := range cmd.ProvidedFields {
		if domain.Contains(domain.UserProtectedUpdateFields, f) && !writable[f] {
			_ = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
				return s.deps.recordAudit(txCtx, p, "user.update", "users", id, user.TenantID,
					nil, nil, nil, domain.AuditDenied, "field_not_allowed:"+f)
			})
			return nil, apperr.FieldNotAllowed(f)
		}
	}

	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		user, err = s.deps.Users.GetByID(txCtx, id)
		if err != nil {
			return apperr.NotFound("User")
		}
		if cmd.Version != 0 && cmd.Version != user.Version {
			return apperr.VersionConflict()
		}
		before := map[string]any{"email": user.Email, "first_name": user.FirstName,
			"last_name": user.LastName, "phone": user.Phone, "internal_notes": user.InternalNotes}
		var changed []string
		if cmd.Email != nil {
			email := strings.TrimSpace(strings.ToLower(*cmd.Email))
			if err := validation.Email(email); err != nil {
				return apperr.Validation(err.Error())
			}
			if email != user.Email {
				if existing, err := s.deps.Users.GetByEmail(txCtx, user.TenantID, email); err == nil && existing != nil && existing.ID != user.ID {
					return apperr.Conflict("A user with this email already exists.")
				}
				user.Email = email
				changed = append(changed, "email")
			}
		}
		if cmd.FirstName != nil && *cmd.FirstName != user.FirstName {
			user.FirstName = strings.TrimSpace(*cmd.FirstName)
			changed = append(changed, "first_name")
		}
		if cmd.LastName != nil && *cmd.LastName != user.LastName {
			user.LastName = strings.TrimSpace(*cmd.LastName)
			changed = append(changed, "last_name")
		}
		if cmd.Phone != nil && *cmd.Phone != user.Phone {
			user.Phone = strings.TrimSpace(*cmd.Phone)
			changed = append(changed, "phone")
		}
		if cmd.InternalNotes != nil && *cmd.InternalNotes != user.InternalNotes {
			user.InternalNotes = *cmd.InternalNotes
			changed = append(changed, "internal_notes")
		}
		if len(changed) == 0 {
			return nil
		}
		user.UpdatedBy = p.ID
		if err := s.deps.Users.Update(txCtx, user); err != nil {
			return err
		}
		after := map[string]any{"email": user.Email, "first_name": user.FirstName,
			"last_name": user.LastName, "phone": user.Phone, "internal_notes": user.InternalNotes}
		if err := s.deps.recordAudit(txCtx, p, "user.updated", "users", user.ID, user.TenantID,
			before, after, changed, domain.AuditSuccess, ""); err != nil {
			return err
		}
		return s.deps.emitEvent(txCtx, p, events.UserUpdatedV1, "user", user.ID, user.TenantID,
			map[string]any{"changed_fields": changed})
	})
	if err != nil {
		return nil, apperr.From(err)
	}
	return &UserView{User: user, AllowedFields: dec.PermSet.AllowedFields(ServiceName, "users", domain.ActionRead, dec.Relation)}, nil
}

func (s *UserService) Delete(ctx context.Context, p domain.Principal, id string) error {
	return s.changeLifecycle(ctx, p, id, domain.ActionDelete, "user.deleted", func(u *domain.User) error {
		if u.ID == p.ID {
			return apperr.Conflict("You cannot delete your own account.")
		}
		return u.TransitionTo(domain.UserDisabled)
	}, true)
}

func (s *UserService) Activate(ctx context.Context, p domain.Principal, id string) error {
	return s.changeLifecycle(ctx, p, id, domain.ActionUpdate, "user.activated", func(u *domain.User) error {
		return u.TransitionTo(domain.UserActive)
	}, false)
}

func (s *UserService) Suspend(ctx context.Context, p domain.Principal, id string) error {
	return s.changeLifecycle(ctx, p, id, domain.ActionUpdate, "user.suspended", func(u *domain.User) error {
		if u.ID == p.ID {
			return apperr.Conflict("You cannot suspend your own account.")
		}
		return u.TransitionTo(domain.UserSuspended)
	}, false)
}

func (s *UserService) Unlock(ctx context.Context, p domain.Principal, id string) error {
	return s.changeLifecycle(ctx, p, id, domain.ActionUpdate, "user.unlocked", func(u *domain.User) error {
		return u.TransitionTo(domain.UserActive)
	}, false)
}

// changeLifecycle centralizes status transitions with authz + audit + events.
func (s *UserService) changeLifecycle(ctx context.Context, p domain.Principal, id string, action domain.Action,
	auditAction string, mutate func(*domain.User) error, softDelete bool) error {
	if err := validation.UUID(id); err != nil {
		return apperr.BadRequest("Invalid user id.")
	}
	var user *domain.User
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		user, err = s.deps.Users.GetByID(txCtx, id)
		return err
	})
	if err != nil {
		return apperr.NotFound("User")
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "users", Action: action, Method: "POST", Path: "/api/v1/users/" + id,
		TargetTenantID: user.TenantID, TargetUserID: user.ID,
	}); err != nil {
		return err
	}
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		user, err = s.deps.Users.GetByID(txCtx, id)
		if err != nil {
			return apperr.NotFound("User")
		}
		before := string(user.Status)
		if err := mutate(user); err != nil {
			return err
		}
		user.UpdatedBy = p.ID
		if err := s.deps.Users.Update(txCtx, user); err != nil {
			return err
		}
		if softDelete {
			if err := s.deps.Users.SoftDelete(txCtx, id, p.ID); err != nil {
				return err
			}
			if err := s.deps.Sessions.RevokeAllForUser(txCtx, id, "user_deleted"); err != nil {
				return err
			}
		}
		if user.Status == domain.UserSuspended || user.Status == domain.UserDisabled {
			if err := s.deps.Sessions.RevokeAllForUser(txCtx, id, "status_"+string(user.Status)); err != nil {
				return err
			}
		}
		if err := s.deps.recordAudit(txCtx, p, auditAction, "users", id, user.TenantID,
			map[string]any{"status": before}, map[string]any{"status": string(user.Status)},
			[]string{"status"}, domain.AuditSuccess, ""); err != nil {
			return err
		}
		return s.deps.emitEvent(txCtx, p, events.UserStatusChangedV1, "user", id, user.TenantID,
			map[string]any{"from": before, "to": string(user.Status)})
	})
	if err != nil {
		return apperr.From(err)
	}
	s.az.InvalidatePermissions(ctx, user.TenantID, id)
	return nil
}

// EffectivePermissions returns another user's computed permission set.
func (s *UserService) EffectivePermissions(ctx context.Context, p domain.Principal, userID string) ([]domain.Permission, error) {
	if err := validation.UUID(userID); err != nil {
		return nil, apperr.BadRequest("Invalid user id.")
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
	if userID != p.ID {
		if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
			Resource: "permissions", Action: domain.ActionRead, Method: "GET",
			Path:           "/api/v1/users/" + userID + "/effective-permissions",
			TargetTenantID: user.TenantID, TargetUserID: userID,
		}); err != nil {
			return nil, err
		}
	}
	var perms []domain.Permission
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		perms, err = s.deps.Permissions.EffectiveForUser(txCtx, userID)
		return err
	})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return perms, nil
}

func fieldSet(fields []string) map[string]bool {
	m := make(map[string]bool, len(fields))
	for _, f := range fields {
		m[f] = true
	}
	return m
}
