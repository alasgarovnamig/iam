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

type TenantService struct {
	deps *Deps
	az   *Authorizer
}

func NewTenantService(d *Deps, az *Authorizer) *TenantService { return &TenantService{deps: d, az: az} }

type CreateTenantCommand struct {
	Name string
	Slug string
}

func (s *TenantService) Create(ctx context.Context, p domain.Principal, cmd CreateTenantCommand) (*domain.Tenant, error) {
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "tenants", Action: domain.ActionCreate, Method: "POST", Path: "/api/v1/tenants",
	}); err != nil {
		return nil, err
	}
	cmd.Name = strings.TrimSpace(cmd.Name)
	cmd.Slug = strings.TrimSpace(strings.ToLower(cmd.Slug))
	if err := validation.Required("name", cmd.Name); err != nil {
		return nil, apperr.Validation(err.Error())
	}
	if err := validation.MaxLen("name", cmd.Name, 200); err != nil {
		return nil, apperr.Validation(err.Error())
	}
	if err := validation.Slug(cmd.Slug); err != nil {
		return nil, apperr.Validation("slug: " + err.Error())
	}

	tenant := &domain.Tenant{
		ID:        newID(),
		Name:      cmd.Name,
		Slug:      cmd.Slug,
		Status:    domain.TenantActive,
		Settings:  map[string]any{},
		CreatedAt: s.deps.now(),
		UpdatedAt: s.deps.now(),
		CreatedBy: p.ID,
		UpdatedBy: p.ID,
		Version:   1,
	}
	err := s.deps.Tx.WithTx(ctx, ports.DBContext{Platform: true}, func(txCtx context.Context) error {
		if existing, err := s.deps.Tenants.GetBySlug(txCtx, tenant.Slug); err == nil && existing != nil {
			return apperr.Conflict("A tenant with this slug already exists.")
		}
		if err := s.deps.Tenants.Create(txCtx, tenant); err != nil {
			return err
		}
		if err := s.deps.recordAudit(txCtx, p, "tenant.created", "tenants", tenant.ID, tenant.ID,
			nil, map[string]any{"name": tenant.Name, "slug": tenant.Slug, "status": string(tenant.Status)},
			[]string{"name", "slug", "status"}, domain.AuditSuccess, ""); err != nil {
			return err
		}
		return s.deps.emitEvent(txCtx, p, events.TenantCreatedV1, "tenant", tenant.ID, tenant.ID,
			map[string]any{"name": tenant.Name, "slug": tenant.Slug, "status": string(tenant.Status)})
	})
	if err != nil {
		return nil, apperr.From(err)
	}
	// Relationship: the platform manages this tenant.
	if err := s.deps.FGA.Write(ctx, []ports.RelationTuple{
		{User: "platform:platform", Relation: "platform", Object: "tenant:" + tenant.ID},
	}, nil); err != nil {
		s.deps.Logger.Error("failed to write tenant FGA tuple", "tenant_id", tenant.ID, "error", err)
	}
	return tenant, nil
}

func (s *TenantService) Get(ctx context.Context, p domain.Principal, id string) (*domain.Tenant, error) {
	if err := validation.UUID(id); err != nil {
		return nil, apperr.BadRequest("Invalid tenant id.")
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "tenants", Action: domain.ActionRead, Method: "GET", Path: "/api/v1/tenants/" + id,
		TargetTenantID: id,
	}); err != nil {
		return nil, err
	}
	var tenant *domain.Tenant
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		tenant, err = s.deps.Tenants.GetByID(txCtx, id)
		return err
	})
	if err != nil {
		return nil, apperr.NotFound("Tenant")
	}
	return tenant, nil
}

func (s *TenantService) List(ctx context.Context, p domain.Principal, page ports.ListPage) ([]*domain.Tenant, int, error) {
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "tenants", Action: domain.ActionList, Method: "GET", Path: "/api/v1/tenants",
	}); err != nil {
		return nil, 0, err
	}
	sanitizeListPage(&page, map[string]bool{"name": true, "slug": true, "status": true, "created_at": true},
		map[string]bool{"status": true, "slug": true})
	var out []*domain.Tenant
	var total int
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		out, total, err = s.deps.Tenants.List(txCtx, page)
		return err
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	return out, total, nil
}

// Search runs a dynamic filter tree over tenants. Requires tenants.search plus
// tenants.search.<field> for every field touched.
func (s *TenantService) Search(ctx context.Context, p domain.Principal, cmd SearchCommand) ([]*domain.Tenant, int, error) {
	dec, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "tenants", Action: domain.ActionSearch, Method: "POST", Path: "/api/v1/tenants/search",
		TargetTenantID: p.TenantID,
	})
	if err != nil {
		return nil, 0, err
	}
	q, err := authorizeSearch(dec, domain.TenantsSearchSchema, cmd)
	if err != nil {
		return nil, 0, err
	}
	var out []*domain.Tenant
	var total int
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		out, total, err = s.deps.Tenants.Search(txCtx, q)
		return err
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	return out, total, nil
}

type UpdateTenantCommand struct {
	Name    *string
	Status  *string
	Version int64
}

func (s *TenantService) Update(ctx context.Context, p domain.Principal, id string, cmd UpdateTenantCommand) (*domain.Tenant, error) {
	if err := validation.UUID(id); err != nil {
		return nil, apperr.BadRequest("Invalid tenant id.")
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "tenants", Action: domain.ActionUpdate, Method: "PATCH", Path: "/api/v1/tenants/" + id,
		TargetTenantID: id,
	}); err != nil {
		return nil, err
	}
	var tenant *domain.Tenant
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		tenant, err = s.deps.Tenants.GetByID(txCtx, id)
		if err != nil {
			return apperr.NotFound("Tenant")
		}
		if cmd.Version != 0 && cmd.Version != tenant.Version {
			return apperr.VersionConflict()
		}
		before := map[string]any{"name": tenant.Name, "status": string(tenant.Status)}
		var changed []string
		if cmd.Name != nil && *cmd.Name != tenant.Name {
			if err := validation.Required("name", *cmd.Name); err != nil {
				return apperr.Validation(err.Error())
			}
			tenant.Name = strings.TrimSpace(*cmd.Name)
			changed = append(changed, "name")
		}
		if cmd.Status != nil && *cmd.Status != string(tenant.Status) {
			// Status transitions are platform-only.
			if !p.IsPlatform() {
				return apperr.PermissionDenied("Only platform administrators can change tenant status.")
			}
			if err := tenant.TransitionTo(domain.TenantStatus(*cmd.Status)); err != nil {
				return err
			}
			changed = append(changed, "status")
		}
		if len(changed) == 0 {
			return nil
		}
		tenant.UpdatedBy = p.ID
		if err := s.deps.Tenants.Update(txCtx, tenant); err != nil {
			return err
		}
		after := map[string]any{"name": tenant.Name, "status": string(tenant.Status)}
		if err := s.deps.recordAudit(txCtx, p, "tenant.updated", "tenants", tenant.ID, tenant.ID,
			before, after, changed, domain.AuditSuccess, ""); err != nil {
			return err
		}
		return s.deps.emitEvent(txCtx, p, events.TenantUpdatedV1, "tenant", tenant.ID, tenant.ID, after)
	})
	if err != nil {
		return nil, apperr.From(err)
	}
	return tenant, nil
}

func (s *TenantService) Delete(ctx context.Context, p domain.Principal, id string) error {
	if err := validation.UUID(id); err != nil {
		return apperr.BadRequest("Invalid tenant id.")
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "tenants", Action: domain.ActionDelete, Method: "DELETE", Path: "/api/v1/tenants/" + id,
		TargetTenantID: id,
	}); err != nil {
		return err
	}
	return s.deps.Tx.WithTx(ctx, ports.DBContext{Platform: true}, func(txCtx context.Context) error {
		tenant, err := s.deps.Tenants.GetByID(txCtx, id)
		if err != nil {
			return apperr.NotFound("Tenant")
		}
		if err := tenant.TransitionTo(domain.TenantTerminated); err != nil {
			return err
		}
		tenant.UpdatedBy = p.ID
		if err := s.deps.Tenants.Update(txCtx, tenant); err != nil {
			return err
		}
		if err := s.deps.Tenants.SoftDelete(txCtx, id, p.ID); err != nil {
			return err
		}
		if err := s.deps.recordAudit(txCtx, p, "tenant.deleted", "tenants", id, id,
			nil, nil, []string{"status", "deleted_at"}, domain.AuditSuccess, ""); err != nil {
			return err
		}
		return s.deps.emitEvent(txCtx, p, events.TenantUpdatedV1, "tenant", id, id,
			map[string]any{"status": string(domain.TenantTerminated)})
	})
}

// sanitizeListPage clamps pagination and applies sort/filter allow-lists.
func sanitizeListPage(page *ports.ListPage, sortable map[string]bool, filterable map[string]bool) {
	if page.Page < 1 {
		page.Page = 1
	}
	if page.PageSize < 1 {
		page.PageSize = 20
	}
	if page.PageSize > 100 {
		page.PageSize = 100
	}
	if page.SortBy == "" || !sortable[page.SortBy] {
		page.SortBy = "created_at"
	}
	if page.SortDir != "asc" {
		page.SortDir = "desc"
	}
	clean := map[string]string{}
	for k, v := range page.Filters {
		if filterable[k] && v != "" {
			clean[k] = v
		}
	}
	page.Filters = clean
}
