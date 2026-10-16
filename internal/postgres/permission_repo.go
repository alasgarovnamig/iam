package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
)

type PermissionRepo struct{}

func NewPermissionRepo() *PermissionRepo { return &PermissionRepo{} }

func (r *PermissionRepo) ListDefinitions(ctx context.Context) ([]*domain.PermissionDefinition, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id, service, resource, action, COALESCE(field,''),
		description, is_platform_only FROM permission_definitions
		ORDER BY service, resource, action, field NULLS FIRST`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.PermissionDefinition
	for rows.Next() {
		d := &domain.PermissionDefinition{}
		if err := rows.Scan(&d.ID, &d.Service, &d.Resource, &d.Action, &d.Field,
			&d.Description, &d.IsPlatformOnly); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *PermissionRepo) GetDefinition(ctx context.Context, id string) (*domain.PermissionDefinition, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	d := &domain.PermissionDefinition{}
	err = tx.QueryRow(ctx, `SELECT id, service, resource, action, COALESCE(field,''),
		description, is_platform_only FROM permission_definitions WHERE id = $1`, id).
		Scan(&d.ID, &d.Service, &d.Resource, &d.Action, &d.Field, &d.Description, &d.IsPlatformOnly)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("Permission definition")
	}
	return d, err
}

func (r *PermissionRepo) FindDefinition(ctx context.Context, service, resource string, action domain.Action, field string) (*domain.PermissionDefinition, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	d := &domain.PermissionDefinition{}
	err = tx.QueryRow(ctx, `SELECT id, service, resource, action, COALESCE(field,''),
		description, is_platform_only FROM permission_definitions
		WHERE service=$1 AND resource=$2 AND action=$3 AND COALESCE(field,'')=$4`,
		service, resource, action, field).
		Scan(&d.ID, &d.Service, &d.Resource, &d.Action, &d.Field, &d.Description, &d.IsPlatformOnly)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("Permission definition")
	}
	return d, err
}

// EffectiveForUser aggregates permissions from active role assignments and
// unexpired direct grants in one round-trip.
func (r *PermissionRepo) EffectiveForUser(ctx context.Context, userID string) ([]domain.Permission, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT pd.id, pd.service, pd.resource, pd.action, COALESCE(pd.field,''), rp.scope, rp.effect, rp.conditions
		FROM user_roles ur
		JOIN roles ro ON ro.id = ur.role_id AND ro.deleted_at IS NULL AND ro.status = 'active'
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		JOIN permission_definitions pd ON pd.id = rp.permission_id
		WHERE ur.user_id = $1
		UNION ALL
		SELECT pd.id, pd.service, pd.resource, pd.action, COALESCE(pd.field,''), up.scope, up.effect, up.conditions
		FROM user_permissions up
		JOIN permission_definitions pd ON pd.id = up.permission_id
		WHERE up.user_id = $1 AND (up.expires_at IS NULL OR up.expires_at > now())`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPermissions(rows)
}

func (r *PermissionRepo) EffectiveForAPIClient(ctx context.Context, clientID string) ([]domain.Permission, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT pd.id, pd.service, pd.resource, pd.action, COALESCE(pd.field,''), acp.scope, acp.effect, acp.conditions
		FROM api_client_permissions acp
		JOIN permission_definitions pd ON pd.id = acp.permission_id
		WHERE acp.api_client_id = $1`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPermissions(rows)
}

func scanPermissions(rows pgx.Rows) ([]domain.Permission, error) {
	var out []domain.Permission
	for rows.Next() {
		var p domain.Permission
		var conds map[string]string
		if err := rows.Scan(&p.DefinitionID, &p.Service, &p.Resource, &p.Action, &p.Field,
			&p.Scope, &p.Effect, &conds); err != nil {
			return nil, err
		}
		p.Conditions = conds
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PermissionRepo) GrantToUser(ctx context.Context, up *domain.UserPermission) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	conds := up.Permission.Conditions
	if conds == nil {
		conds = map[string]string{}
	}
	_, err = tx.Exec(ctx, `INSERT INTO user_permissions (id, user_id, tenant_id, permission_id,
		scope, effect, conditions, granted_by, created_at, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (user_id, permission_id, scope, effect) DO NOTHING`,
		up.ID, up.UserID, nullable(up.TenantID), up.Permission.DefinitionID,
		up.Permission.Scope, up.Permission.Effect, conds, nullable(up.GrantedBy),
		up.CreatedAt, up.ExpiresAt)
	return err
}

func (r *PermissionRepo) RevokeFromUser(ctx context.Context, userID, userPermissionID string) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM user_permissions WHERE id = $1 AND user_id = $2`,
		userPermissionID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("User permission")
	}
	return nil
}

func (r *PermissionRepo) ListUserDirect(ctx context.Context, userID string) ([]*domain.UserPermission, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT up.id, up.user_id, COALESCE(up.tenant_id::text,''),
		pd.id, pd.service, pd.resource, pd.action, COALESCE(pd.field,''), up.scope, up.effect,
		up.conditions, COALESCE(up.granted_by::text,''), up.created_at, up.expires_at
		FROM user_permissions up
		JOIN permission_definitions pd ON pd.id = up.permission_id
		WHERE up.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.UserPermission
	for rows.Next() {
		up := &domain.UserPermission{}
		var conds map[string]string
		if err := rows.Scan(&up.ID, &up.UserID, &up.TenantID,
			&up.Permission.DefinitionID, &up.Permission.Service, &up.Permission.Resource,
			&up.Permission.Action, &up.Permission.Field, &up.Permission.Scope, &up.Permission.Effect,
			&conds, &up.GrantedBy, &up.CreatedAt, &up.ExpiresAt); err != nil {
			return nil, err
		}
		up.Permission.Conditions = conds
		out = append(out, up)
	}
	return out, rows.Err()
}
