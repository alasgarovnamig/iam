package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
)

type TenantRepo struct{}

func NewTenantRepo() *TenantRepo { return &TenantRepo{} }

const tenantColumns = `id, name, slug, status, settings, created_at, updated_at,
	COALESCE(created_by::text,''), COALESCE(updated_by::text,''), version, deleted_at`

func scanTenant(row pgx.Row) (*domain.Tenant, error) {
	t := &domain.Tenant{}
	err := row.Scan(&t.ID, &t.Name, &t.Slug, &t.Status, &t.Settings, &t.CreatedAt, &t.UpdatedAt,
		&t.CreatedBy, &t.UpdatedBy, &t.Version, &t.DeletedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (r *TenantRepo) Create(ctx context.Context, t *domain.Tenant) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO tenants (id, name, slug, status, settings, created_at,
		updated_at, created_by, updated_by, version) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		t.ID, t.Name, t.Slug, t.Status, t.Settings, t.CreatedAt, t.UpdatedAt,
		nullable(t.CreatedBy), nullable(t.UpdatedBy), t.Version)
	if err != nil && strings.Contains(err.Error(), "ux_tenants_slug") {
		return apperr.Conflict("A tenant with this slug already exists.")
	}
	return err
}

func (r *TenantRepo) GetByID(ctx context.Context, id string) (*domain.Tenant, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	t, err := scanTenant(tx.QueryRow(ctx,
		`SELECT `+tenantColumns+` FROM tenants WHERE id = $1 AND deleted_at IS NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("Tenant")
	}
	return t, err
}

func (r *TenantRepo) GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	t, err := scanTenant(tx.QueryRow(ctx,
		`SELECT `+tenantColumns+` FROM tenants WHERE slug = $1 AND deleted_at IS NULL`, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("Tenant")
	}
	return t, err
}

func (r *TenantRepo) Update(ctx context.Context, t *domain.Tenant) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE tenants SET name=$2, status=$3, settings=$4,
		updated_by=$5, version = version + 1
		WHERE id=$1 AND version=$6 AND deleted_at IS NULL`,
		t.ID, t.Name, t.Status, t.Settings, nullable(t.UpdatedBy), t.Version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.VersionConflict()
	}
	t.Version++
	return nil
}

func (r *TenantRepo) SoftDelete(ctx context.Context, id string, by string) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE tenants SET deleted_at = now(), updated_by = $2
		WHERE id = $1 AND deleted_at IS NULL`, id, nullable(by))
	return err
}

func (r *TenantRepo) List(ctx context.Context, page ports.ListPage) ([]*domain.Tenant, int, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, 0, err
	}
	where := []string{"deleted_at IS NULL"}
	args := []any{}
	i := 1
	for field, value := range page.Filters {
		switch field {
		case "status", "slug":
			where = append(where, fmt.Sprintf("%s = $%d", field, i))
			args = append(args, value)
			i++
		}
	}
	if page.Search != "" {
		where = append(where, fmt.Sprintf("(name ILIKE $%d OR slug ILIKE $%d)", i, i))
		args = append(args, "%"+page.Search+"%")
		i++
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tenants WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	sortCol := map[string]string{"name": "name", "slug": "slug", "status": "status", "created_at": "created_at"}[page.SortBy]
	if sortCol == "" {
		sortCol = "created_at"
	}
	dir := "DESC"
	if page.SortDir == "asc" {
		dir = "ASC"
	}
	query := fmt.Sprintf(`SELECT %s FROM tenants WHERE %s ORDER BY %s %s, id %s LIMIT $%d OFFSET $%d`,
		tenantColumns, whereSQL, sortCol, dir, dir, i, i+1)
	args = append(args, page.PageSize, page.Offset())
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.Tenant
	for rows.Next() {
		t, err := scanTenant(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

// Search runs a validated dynamic filter tree against tenants. Row-Level
// Security still limits a tenant principal to its own row.
func (r *TenantRepo) Search(ctx context.Context, q domain.SearchQuery) ([]*domain.Tenant, int, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, 0, err
	}
	whereSQL, args, next, err := buildWhere(domain.TenantsSearchSchema, q, []string{"deleted_at IS NULL"}, nil, 1)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tenants WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := fmt.Sprintf(`SELECT %s FROM tenants WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d`,
		tenantColumns, whereSQL, orderClause(domain.TenantsSearchSchema, q), next, next+1)
	args = append(args, q.PageSize, q.Offset())
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.Tenant
	for rows.Next() {
		t, err := scanTenant(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}
