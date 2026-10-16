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

type RoleRepo struct{}

func NewRoleRepo() *RoleRepo { return &RoleRepo{} }

const roleColumns = `id, COALESCE(tenant_id::text,''), name, description, role_type, is_system,
	status, COALESCE(parent_role_id::text,''), created_at, updated_at,
	COALESCE(created_by::text,''), COALESCE(updated_by::text,''), version, deleted_at`

func scanRole(row pgx.Row) (*domain.Role, error) {
	r := &domain.Role{}
	err := row.Scan(&r.ID, &r.TenantID, &r.Name, &r.Description, &r.Type, &r.IsSystem, &r.Status,
		&r.ParentRoleID, &r.CreatedAt, &r.UpdatedAt, &r.CreatedBy, &r.UpdatedBy, &r.Version, &r.DeletedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (r *RoleRepo) Create(ctx context.Context, role *domain.Role) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO roles (id, tenant_id, name, description, role_type,
		is_system, status, parent_role_id, created_at, updated_at, created_by, updated_by, version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		role.ID, nullable(role.TenantID), role.Name, role.Description, role.Type, role.IsSystem,
		role.Status, nullable(role.ParentRoleID), role.CreatedAt, role.UpdatedAt,
		nullable(role.CreatedBy), nullable(role.UpdatedBy), role.Version)
	if err != nil && strings.Contains(err.Error(), "ux_roles_tenant_name") {
		return apperr.Conflict("A role with this name already exists.")
	}
	return err
}

func (r *RoleRepo) GetByID(ctx context.Context, id string) (*domain.Role, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	role, err := scanRole(tx.QueryRow(ctx,
		`SELECT `+roleColumns+` FROM roles WHERE id = $1 AND deleted_at IS NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("Role")
	}
	return role, err
}

func (r *RoleRepo) Update(ctx context.Context, role *domain.Role) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE roles SET name=$2, description=$3, status=$4,
		updated_by=$5, version = version + 1
		WHERE id=$1 AND version=$6 AND deleted_at IS NULL AND is_system = false`,
		role.ID, role.Name, role.Description, role.Status, nullable(role.UpdatedBy), role.Version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.VersionConflict()
	}
	role.Version++
	return nil
}

func (r *RoleRepo) SoftDelete(ctx context.Context, id string, by string) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE roles SET deleted_at = now(), updated_by = $2
		WHERE id = $1 AND deleted_at IS NULL AND is_system = false`, id, nullable(by))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("Role")
	}
	return nil
}

func (r *RoleRepo) List(ctx context.Context, tenantID string, includeSystem bool, page ports.ListPage) ([]*domain.Role, int, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, 0, err
	}
	where := []string{"deleted_at IS NULL"}
	args := []any{}
	i := 1
	if tenantID == "" {
		// Platform view: system roles + platform custom roles.
		where = append(where, "tenant_id IS NULL")
	} else if includeSystem {
		where = append(where, fmt.Sprintf("(tenant_id = $%d OR (tenant_id IS NULL AND is_system AND role_type = 'tenant'))", i))
		args = append(args, tenantID)
		i++
	} else {
		where = append(where, fmt.Sprintf("tenant_id = $%d", i))
		args = append(args, tenantID)
		i++
	}
	for field, value := range page.Filters {
		switch field {
		case "role_type", "status":
			where = append(where, fmt.Sprintf("%s = $%d", field, i))
			args = append(args, value)
			i++
		}
	}
	if page.Search != "" {
		where = append(where, fmt.Sprintf("name ILIKE $%d", i))
		args = append(args, "%"+page.Search+"%")
		i++
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM roles WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	dir := "DESC"
	if page.SortDir == "asc" {
		dir = "ASC"
	}
	sortCol := "created_at"
	if page.SortBy == "name" {
		sortCol = "name"
	}
	query := fmt.Sprintf(`SELECT %s FROM roles WHERE %s ORDER BY is_system DESC, %s %s, id LIMIT $%d OFFSET $%d`,
		roleColumns, whereSQL, sortCol, dir, i, i+1)
	args = append(args, page.PageSize, page.Offset())
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.Role
	for rows.Next() {
		role, err := scanRole(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, role)
	}
	return out, total, rows.Err()
}

const rolePermColumns = `rp.id, rp.role_id, COALESCE(rp.tenant_id::text,''), rp.scope, rp.effect,
	rp.conditions, COALESCE(rp.created_by::text,''), rp.created_at,
	pd.id, pd.service, pd.resource, pd.action, COALESCE(pd.field,'')`

func scanRolePermission(row pgx.Row) (*domain.RolePermission, error) {
	rp := &domain.RolePermission{}
	var conds map[string]string
	err := row.Scan(&rp.ID, &rp.RoleID, &rp.TenantID, &rp.Permission.Scope, &rp.Permission.Effect,
		&conds, &rp.CreatedBy, &rp.CreatedAt,
		&rp.Permission.DefinitionID, &rp.Permission.Service, &rp.Permission.Resource,
		&rp.Permission.Action, &rp.Permission.Field)
	if err != nil {
		return nil, err
	}
	rp.Permission.Conditions = conds
	return rp, nil
}

func (r *RoleRepo) AddPermission(ctx context.Context, rp *domain.RolePermission) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	conds := rp.Permission.Conditions
	if conds == nil {
		conds = map[string]string{}
	}
	_, err = tx.Exec(ctx, `INSERT INTO role_permissions (id, role_id, tenant_id, permission_id,
		scope, effect, conditions, created_by, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (role_id, permission_id, scope, effect) DO NOTHING`,
		rp.ID, rp.RoleID, nullable(rp.TenantID), rp.Permission.DefinitionID,
		rp.Permission.Scope, rp.Permission.Effect, conds, nullable(rp.CreatedBy), rp.CreatedAt)
	return err
}

func (r *RoleRepo) RemovePermission(ctx context.Context, roleID, rolePermissionID string) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE id = $1 AND role_id = $2`,
		rolePermissionID, roleID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("Role permission")
	}
	return nil
}

func (r *RoleRepo) ListPermissions(ctx context.Context, roleID string) ([]*domain.RolePermission, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT `+rolePermColumns+` FROM role_permissions rp
		JOIN permission_definitions pd ON pd.id = rp.permission_id
		WHERE rp.role_id = $1 ORDER BY pd.resource, pd.action, pd.field NULLS FIRST`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.RolePermission
	for rows.Next() {
		rp, err := scanRolePermission(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rp)
	}
	return out, rows.Err()
}

func (r *RoleRepo) AssignToUser(ctx context.Context, ur *domain.UserRole) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO user_roles (id, user_id, role_id, tenant_id, assigned_by, created_at)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (user_id, role_id) DO NOTHING`,
		ur.ID, ur.UserID, ur.RoleID, nullable(ur.TenantID), nullable(ur.AssignedBy), ur.CreatedAt)
	return err
}

func (r *RoleRepo) RevokeFromUser(ctx context.Context, userID, roleID string) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2`, userID, roleID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("Role assignment")
	}
	return nil
}

func (r *RoleRepo) ListUserRoles(ctx context.Context, userID string) ([]*domain.Role, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT roles.id, COALESCE(roles.tenant_id::text,''), roles.name,
		roles.description, roles.role_type, roles.is_system, roles.status,
		COALESCE(roles.parent_role_id::text,''), roles.created_at, roles.updated_at,
		COALESCE(roles.created_by::text,''), COALESCE(roles.updated_by::text,''),
		roles.version, roles.deleted_at
		FROM roles
		JOIN user_roles ur ON ur.role_id = roles.id
		WHERE ur.user_id = $1 AND roles.deleted_at IS NULL AND roles.status = 'active'`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Role
	for rows.Next() {
		role, err := scanRole(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, role)
	}
	return out, rows.Err()
}

func (r *RoleRepo) CountUsersWithRole(ctx context.Context, roleID string) (int, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return 0, err
	}
	var n int
	err = tx.QueryRow(ctx, `SELECT count(*) FROM user_roles WHERE role_id = $1`, roleID).Scan(&n)
	return n, err
}
