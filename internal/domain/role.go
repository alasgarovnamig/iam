package domain

import (
	"time"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
)

type RoleType string

const (
	RoleTypePlatform RoleType = "platform"
	RoleTypeTenant   RoleType = "tenant"
)

type Role struct {
	ID           string
	TenantID     string // empty = system role
	Name         string
	Description  string
	Type         RoleType
	IsSystem     bool
	Status       string // active | disabled
	ParentRoleID string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CreatedBy    string
	UpdatedBy    string
	Version      int64
	DeletedAt    *time.Time
}

// EnsureMutable rejects modification of system roles.
func (r *Role) EnsureMutable() error {
	if r.IsSystem {
		return apperr.PermissionDenied("System roles cannot be modified.")
	}
	return nil
}

// RolePermission links a role to a permission definition with scope/effect.
type RolePermission struct {
	ID         string
	RoleID     string
	TenantID   string
	Permission Permission
	CreatedBy  string
	CreatedAt  time.Time
}

// UserRole assigns a role to a user.
type UserRole struct {
	ID         string
	UserID     string
	RoleID     string
	TenantID   string
	AssignedBy string
	CreatedAt  time.Time
}

// UserPermission is a direct grant to a user.
type UserPermission struct {
	ID         string
	UserID     string
	TenantID   string
	Permission Permission
	GrantedBy  string
	CreatedAt  time.Time
	ExpiresAt  *time.Time
}

// PermissionDefinition is a catalog entry.
type PermissionDefinition struct {
	ID             string
	Service        string
	Resource       string
	Action         Action
	Field          string
	Description    string
	IsPlatformOnly bool
}

func (d PermissionDefinition) Code() string {
	p := Permission{Service: d.Service, Resource: d.Resource, Action: d.Action, Field: d.Field}
	return p.Code()
}
