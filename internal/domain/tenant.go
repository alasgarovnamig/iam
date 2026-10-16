package domain

import (
	"time"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
)

type TenantStatus string

const (
	TenantPending    TenantStatus = "pending"
	TenantActive     TenantStatus = "active"
	TenantSuspended  TenantStatus = "suspended"
	TenantTerminated TenantStatus = "terminated"
)

type Tenant struct {
	ID        string
	Name      string
	Slug      string
	Status    TenantStatus
	Settings  map[string]any
	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy string
	UpdatedBy string
	Version   int64
	DeletedAt *time.Time
}

var validTenantTransitions = map[TenantStatus][]TenantStatus{
	TenantPending:    {TenantActive, TenantTerminated},
	TenantActive:     {TenantSuspended, TenantTerminated},
	TenantSuspended:  {TenantActive, TenantTerminated},
	TenantTerminated: {},
}

func (t *Tenant) TransitionTo(next TenantStatus) error {
	if t.Status == next {
		return nil
	}
	for _, allowed := range validTenantTransitions[t.Status] {
		if allowed == next {
			t.Status = next
			return nil
		}
	}
	return apperr.Conflict("Invalid tenant status transition from " + string(t.Status) + " to " + string(next))
}

func (t *Tenant) IsOperational() bool {
	return t.Status == TenantActive || t.Status == TenantPending
}

// OrganizationUnit is a node in a tenant's org tree.
type OrganizationUnit struct {
	ID        string
	TenantID  string
	ParentID  string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int64
}
