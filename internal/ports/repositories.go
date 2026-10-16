// Package ports defines the interfaces (hexagonal ports) between the
// application core and the outside world. Adapters implement them.
package ports

import (
	"context"
	"time"

	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
)

// DBContext selects the Row-Level Security context for a unit of work.
// Platform=true sets app.is_platform; otherwise app.tenant_id is set.
type DBContext struct {
	Platform bool
	TenantID string
}

// TxManager runs a function inside a database transaction with the RLS
// context applied via SET LOCAL. Repositories obtain the transaction from ctx.
type TxManager interface {
	WithTx(ctx context.Context, dbc DBContext, fn func(ctx context.Context) error) error
}

// ListPage is the common pagination request. Filters are allow-listed by the
// application layer before reaching repositories.
type ListPage struct {
	Page     int
	PageSize int
	SortBy   string
	SortDir  string // asc | desc
	Filters  map[string]string
	Search   string
}

func (p ListPage) Offset() int { return (p.Page - 1) * p.PageSize }

type TenantRepo interface {
	Create(ctx context.Context, t *domain.Tenant) error
	GetByID(ctx context.Context, id string) (*domain.Tenant, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error)
	Update(ctx context.Context, t *domain.Tenant) error // optimistic (version)
	SoftDelete(ctx context.Context, id string, by string) error
	List(ctx context.Context, page ListPage) ([]*domain.Tenant, int, error)
	Search(ctx context.Context, q domain.SearchQuery) ([]*domain.Tenant, int, error)
}

type UserRepo interface {
	Create(ctx context.Context, u *domain.User) error
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByEmail(ctx context.Context, tenantID, email string) (*domain.User, error)
	// FindByEmailAny returns all non-deleted users with the email across
	// tenants (platform DB context only; used for login resolution).
	FindByEmailAny(ctx context.Context, email string) ([]*domain.User, error)
	Update(ctx context.Context, u *domain.User) error // optimistic (version)
	SoftDelete(ctx context.Context, id string, by string) error
	List(ctx context.Context, tenantID string, page ListPage) ([]*domain.User, int, error)
	// Search runs a validated dynamic filter tree, scoped to tenantID ("" =
	// platform, all tenants) on top of Row-Level Security.
	Search(ctx context.Context, tenantID string, q domain.SearchQuery) ([]*domain.User, int, error)
}

type CredentialRepo interface {
	Create(ctx context.Context, c *domain.Credential) error
	GetByUserID(ctx context.Context, userID string) (*domain.Credential, error)
	Update(ctx context.Context, c *domain.Credential) error
}

type SessionRepo interface {
	Create(ctx context.Context, s *domain.Session) error
	GetByTokenHash(ctx context.Context, hash string) (*domain.Session, error)
	GetByID(ctx context.Context, id string) (*domain.Session, error)
	Update(ctx context.Context, s *domain.Session) error
	ListActiveByUser(ctx context.Context, userID string) ([]*domain.Session, error)
	RevokeFamily(ctx context.Context, familyID, reason string) error
	RevokeAllForUser(ctx context.Context, userID, reason string) error
}

type RoleRepo interface {
	Create(ctx context.Context, r *domain.Role) error
	GetByID(ctx context.Context, id string) (*domain.Role, error)
	Update(ctx context.Context, r *domain.Role) error // optimistic (version)
	SoftDelete(ctx context.Context, id string, by string) error
	List(ctx context.Context, tenantID string, includeSystem bool, page ListPage) ([]*domain.Role, int, error)
	AddPermission(ctx context.Context, rp *domain.RolePermission) error
	RemovePermission(ctx context.Context, roleID, rolePermissionID string) error
	ListPermissions(ctx context.Context, roleID string) ([]*domain.RolePermission, error)
	AssignToUser(ctx context.Context, ur *domain.UserRole) error
	RevokeFromUser(ctx context.Context, userID, roleID string) error
	ListUserRoles(ctx context.Context, userID string) ([]*domain.Role, error)
	CountUsersWithRole(ctx context.Context, roleID string) (int, error)
}

type PermissionRepo interface {
	ListDefinitions(ctx context.Context) ([]*domain.PermissionDefinition, error)
	GetDefinition(ctx context.Context, id string) (*domain.PermissionDefinition, error)
	FindDefinition(ctx context.Context, service, resource string, action domain.Action, field string) (*domain.PermissionDefinition, error)
	// EffectiveForUser aggregates role permissions + direct grants (unexpired).
	EffectiveForUser(ctx context.Context, userID string) ([]domain.Permission, error)
	EffectiveForAPIClient(ctx context.Context, clientID string) ([]domain.Permission, error)
	GrantToUser(ctx context.Context, up *domain.UserPermission) error
	RevokeFromUser(ctx context.Context, userID, userPermissionID string) error
	ListUserDirect(ctx context.Context, userID string) ([]*domain.UserPermission, error)
}

type APIClientRepo interface {
	Create(ctx context.Context, c *domain.APIClient) error
	GetByID(ctx context.Context, id string) (*domain.APIClient, error)
	GetByClientID(ctx context.Context, clientID string) (*domain.APIClient, error)
	Update(ctx context.Context, c *domain.APIClient) error // optimistic (version)
	SoftDelete(ctx context.Context, id string, by string) error
	List(ctx context.Context, tenantID string, page ListPage) ([]*domain.APIClient, int, error)
	Search(ctx context.Context, tenantID string, q domain.SearchQuery) ([]*domain.APIClient, int, error)
	TouchLastUsed(ctx context.Context, id string, at time.Time) error
	GrantPermission(ctx context.Context, p *domain.APIClientPermission) error
	RevokePermission(ctx context.Context, clientID, permissionRowID string) error
	ListPermissions(ctx context.Context, clientID string) ([]*domain.APIClientPermission, error)
}

type AuditRepo interface {
	Append(ctx context.Context, entry *domain.AuditLog) error
	GetByID(ctx context.Context, id string) (*domain.AuditLog, error)
	List(ctx context.Context, tenantID string, page ListPage) ([]*domain.AuditLog, int, error)
}

type OutboxRepo interface {
	Append(ctx context.Context, ev *domain.OutboxEvent) error
	// FetchBatch locks and returns due events (FOR UPDATE SKIP LOCKED).
	FetchBatch(ctx context.Context, limit int, now time.Time) ([]*domain.OutboxEvent, error)
	MarkPublished(ctx context.Context, id int64, at time.Time) error
	MarkFailed(ctx context.Context, id int64, attempt int, nextAttempt time.Time, lastError string) error
	MarkDead(ctx context.Context, id int64, lastError string) error
}
