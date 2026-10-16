package app

import (
	"context"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
	"github.com/sr-alasgarov/wallet-gateway/internal/validation"
)

// AuditService is the read side of audit logs (append happens inside other
// services' transactions).
type AuditService struct {
	deps *Deps
	az   *Authorizer
}

func NewAuditService(d *Deps, az *Authorizer) *AuditService { return &AuditService{deps: d, az: az} }

func (s *AuditService) List(ctx context.Context, p domain.Principal, tenantID string, page ports.ListPage) ([]*domain.AuditLog, int, error) {
	targetTenant := tenantID
	if !p.IsPlatform() {
		targetTenant = p.TenantID
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "audit-logs", Action: domain.ActionList, Method: "GET", Path: "/api/v1/audit-logs",
		TargetTenantID: targetTenant,
	}); err != nil {
		return nil, 0, err
	}
	sanitizeListPage(&page, map[string]bool{"occurred_at": true},
		map[string]bool{"action": true, "resource": true, "actor_id": true, "result": true})
	page.SortBy = "occurred_at"
	var out []*domain.AuditLog
	var total int
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		out, total, err = s.deps.Audit.List(txCtx, targetTenant, page)
		return err
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	return out, total, nil
}

func (s *AuditService) Get(ctx context.Context, p domain.Principal, id string) (*domain.AuditLog, error) {
	if err := validation.UUID(id); err != nil {
		return nil, apperr.BadRequest("Invalid audit log id.")
	}
	var entry *domain.AuditLog
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		entry, err = s.deps.Audit.GetByID(txCtx, id)
		return err
	})
	if err != nil {
		return nil, apperr.NotFound("Audit log")
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "audit-logs", Action: domain.ActionRead, Method: "GET", Path: "/api/v1/audit-logs/" + id,
		TargetTenantID: entry.TenantID,
	}); err != nil {
		return nil, err
	}
	return entry, nil
}
