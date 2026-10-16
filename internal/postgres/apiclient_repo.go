package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
)

type APIClientRepo struct{}

func NewAPIClientRepo() *APIClientRepo { return &APIClientRepo{} }

const apiClientColumns = `id, tenant_id, name, description, client_id, secret_hash, allowed_ips,
	scopes, status, expires_at, last_used_at, secret_rotated_at, created_at, updated_at,
	COALESCE(created_by::text,''), COALESCE(updated_by::text,''), version, deleted_at`

func scanAPIClient(row pgx.Row) (*domain.APIClient, error) {
	c := &domain.APIClient{}
	err := row.Scan(&c.ID, &c.TenantID, &c.Name, &c.Description, &c.ClientID, &c.SecretHash,
		&c.AllowedIPs, &c.Scopes, &c.Status, &c.ExpiresAt, &c.LastUsedAt, &c.SecretRotatedAt,
		&c.CreatedAt, &c.UpdatedAt, &c.CreatedBy, &c.UpdatedBy, &c.Version, &c.DeletedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *APIClientRepo) Create(ctx context.Context, c *domain.APIClient) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	if c.AllowedIPs == nil {
		c.AllowedIPs = []string{}
	}
	if c.Scopes == nil {
		c.Scopes = []string{}
	}
	_, err = tx.Exec(ctx, `INSERT INTO api_clients (id, tenant_id, name, description, client_id,
		secret_hash, allowed_ips, scopes, status, expires_at, created_at, updated_at,
		created_by, updated_by, version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		c.ID, c.TenantID, c.Name, c.Description, c.ClientID, c.SecretHash, c.AllowedIPs,
		c.Scopes, c.Status, c.ExpiresAt, c.CreatedAt, c.UpdatedAt,
		nullable(c.CreatedBy), nullable(c.UpdatedBy), c.Version)
	return err
}

func (r *APIClientRepo) GetByID(ctx context.Context, id string) (*domain.APIClient, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	c, err := scanAPIClient(tx.QueryRow(ctx,
		`SELECT `+apiClientColumns+` FROM api_clients WHERE id = $1 AND deleted_at IS NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("API client")
	}
	return c, err
}

func (r *APIClientRepo) GetByClientID(ctx context.Context, clientID string) (*domain.APIClient, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	c, err := scanAPIClient(tx.QueryRow(ctx,
		`SELECT `+apiClientColumns+` FROM api_clients WHERE client_id = $1 AND deleted_at IS NULL`, clientID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("API client")
	}
	return c, err
}

func (r *APIClientRepo) Update(ctx context.Context, c *domain.APIClient) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE api_clients SET name=$2, description=$3, allowed_ips=$4,
		scopes=$5, status=$6, expires_at=$7, secret_hash=$8, secret_rotated_at=$9,
		updated_by=$10, version = version + 1
		WHERE id=$1 AND version=$11 AND deleted_at IS NULL`,
		c.ID, c.Name, c.Description, c.AllowedIPs, c.Scopes, c.Status, c.ExpiresAt,
		c.SecretHash, c.SecretRotatedAt, nullable(c.UpdatedBy), c.Version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.VersionConflict()
	}
	c.Version++
	return nil
}

func (r *APIClientRepo) SoftDelete(ctx context.Context, id string, by string) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE api_clients SET deleted_at = now(), updated_by = $2
		WHERE id = $1 AND deleted_at IS NULL`, id, nullable(by))
	return err
}

func (r *APIClientRepo) List(ctx context.Context, tenantID string, page ports.ListPage) ([]*domain.APIClient, int, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, 0, err
	}
	where := []string{"deleted_at IS NULL"}
	args := []any{}
	i := 1
	if tenantID != "" {
		where = append(where, fmt.Sprintf("tenant_id = $%d", i))
		args = append(args, tenantID)
		i++
	}
	if v, ok := page.Filters["status"]; ok {
		where = append(where, fmt.Sprintf("status = $%d", i))
		args = append(args, v)
		i++
	}
	if page.Search != "" {
		where = append(where, fmt.Sprintf("name ILIKE $%d", i))
		args = append(args, "%"+page.Search+"%")
		i++
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM api_clients WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	dir := "DESC"
	if page.SortDir == "asc" {
		dir = "ASC"
	}
	sortCol := map[string]string{"name": "name", "status": "status", "created_at": "created_at"}[page.SortBy]
	if sortCol == "" {
		sortCol = "created_at"
	}
	query := fmt.Sprintf(`SELECT %s FROM api_clients WHERE %s ORDER BY %s %s, id LIMIT $%d OFFSET $%d`,
		apiClientColumns, whereSQL, sortCol, dir, i, i+1)
	args = append(args, page.PageSize, page.Offset())
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.APIClient
	for rows.Next() {
		c, err := scanAPIClient(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// Search runs a validated dynamic filter tree against api_clients, scoped to
// tenantID plus Row-Level Security.
func (r *APIClientRepo) Search(ctx context.Context, tenantID string, q domain.SearchQuery) ([]*domain.APIClient, int, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, 0, err
	}
	base := []string{"deleted_at IS NULL"}
	var baseArgs []any
	if tenantID != "" {
		base = append(base, "tenant_id = $1")
		baseArgs = append(baseArgs, tenantID)
	}
	whereSQL, args, next, err := buildWhere(domain.APIClientsSearchSchema, q, base, baseArgs, len(baseArgs)+1)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM api_clients WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := fmt.Sprintf(`SELECT %s FROM api_clients WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d`,
		apiClientColumns, whereSQL, orderClause(domain.APIClientsSearchSchema, q), next, next+1)
	args = append(args, q.PageSize, q.Offset())
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.APIClient
	for rows.Next() {
		c, err := scanAPIClient(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

func (r *APIClientRepo) TouchLastUsed(ctx context.Context, id string, at time.Time) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE api_clients SET last_used_at = $2 WHERE id = $1`, id, at)
	return err
}

func (r *APIClientRepo) GrantPermission(ctx context.Context, p *domain.APIClientPermission) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	conds := p.Permission.Conditions
	if conds == nil {
		conds = map[string]string{}
	}
	_, err = tx.Exec(ctx, `INSERT INTO api_client_permissions (id, api_client_id, tenant_id,
		permission_id, scope, effect, conditions, granted_by, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (api_client_id, permission_id, scope, effect) DO NOTHING`,
		p.ID, p.APIClientID, p.TenantID, p.Permission.DefinitionID,
		p.Permission.Scope, p.Permission.Effect, conds, nullable(p.GrantedBy), p.CreatedAt)
	return err
}

func (r *APIClientRepo) RevokePermission(ctx context.Context, clientID, permissionRowID string) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM api_client_permissions WHERE id = $1 AND api_client_id = $2`,
		permissionRowID, clientID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("API client permission")
	}
	return nil
}

func (r *APIClientRepo) ListPermissions(ctx context.Context, clientID string) ([]*domain.APIClientPermission, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT acp.id, acp.api_client_id, acp.tenant_id,
		pd.id, pd.service, pd.resource, pd.action, COALESCE(pd.field,''), acp.scope, acp.effect,
		acp.conditions, COALESCE(acp.granted_by::text,''), acp.created_at
		FROM api_client_permissions acp
		JOIN permission_definitions pd ON pd.id = acp.permission_id
		WHERE acp.api_client_id = $1`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.APIClientPermission
	for rows.Next() {
		p := &domain.APIClientPermission{}
		var conds map[string]string
		if err := rows.Scan(&p.ID, &p.APIClientID, &p.TenantID,
			&p.Permission.DefinitionID, &p.Permission.Service, &p.Permission.Resource,
			&p.Permission.Action, &p.Permission.Field, &p.Permission.Scope, &p.Permission.Effect,
			&conds, &p.GrantedBy, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.Permission.Conditions = conds
		out = append(out, p)
	}
	return out, rows.Err()
}
