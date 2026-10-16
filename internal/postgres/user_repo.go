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

type UserRepo struct{}

func NewUserRepo() *UserRepo { return &UserRepo{} }

const userColumns = `id, COALESCE(tenant_id::text,''), COALESCE(organization_unit_id::text,''), user_type,
	email, first_name, last_name, COALESCE(phone,''), status, must_change_password,
	COALESCE(internal_notes,''), security_flags, created_at, updated_at,
	COALESCE(created_by::text,''), COALESCE(updated_by::text,''), version, deleted_at`

func scanUser(row pgx.Row) (*domain.User, error) {
	u := &domain.User{}
	var securityFlags map[string]any
	err := row.Scan(&u.ID, &u.TenantID, &u.OrganizationUnitID, &u.Type, &u.Email, &u.FirstName,
		&u.LastName, &u.Phone, &u.Status, &u.MustChangePassword, &u.InternalNotes,
		&securityFlags, &u.CreatedAt, &u.UpdatedAt, &u.CreatedBy, &u.UpdatedBy, &u.Version, &u.DeletedAt)
	if err != nil {
		return nil, err
	}
	u.SecurityFlags = securityFlags
	return u, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *UserRepo) Create(ctx context.Context, u *domain.User) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO users (id, tenant_id, organization_unit_id, user_type, email, first_name,
			last_name, phone, status, must_change_password, internal_notes, security_flags,
			created_at, updated_at, created_by, updated_by, version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		u.ID, nullable(u.TenantID), nullable(u.OrganizationUnitID), u.Type, u.Email, u.FirstName,
		u.LastName, nullable(u.Phone), u.Status, u.MustChangePassword, nullable(u.InternalNotes),
		u.SecurityFlags, u.CreatedAt, u.UpdatedAt, nullable(u.CreatedBy), nullable(u.UpdatedBy), u.Version)
	if err != nil && strings.Contains(err.Error(), "ux_users_tenant_email") {
		return apperr.Conflict("A user with this email already exists.")
	}
	return err
}

func (r *UserRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	u, err := scanUser(tx.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1 AND deleted_at IS NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("User")
	}
	return u, err
}

func (r *UserRepo) GetByEmail(ctx context.Context, tenantID, email string) (*domain.User, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	var row pgx.Row
	if tenantID == "" {
		row = tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users
			WHERE tenant_id IS NULL AND email = $1 AND deleted_at IS NULL`, email)
	} else {
		row = tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users
			WHERE tenant_id = $1 AND email = $2 AND deleted_at IS NULL`, tenantID, email)
	}
	u, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("User")
	}
	return u, err
}

func (r *UserRepo) FindByEmailAny(ctx context.Context, email string) ([]*domain.User, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT `+userColumns+` FROM users
		WHERE email = $1 AND deleted_at IS NULL ORDER BY tenant_id NULLS FIRST`, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *UserRepo) Update(ctx context.Context, u *domain.User) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE users SET email=$2, first_name=$3, last_name=$4, phone=$5, status=$6,
			must_change_password=$7, internal_notes=$8, security_flags=$9,
			updated_by=$10, version = version + 1
		WHERE id=$1 AND version=$11 AND deleted_at IS NULL`,
		u.ID, u.Email, u.FirstName, u.LastName, nullable(u.Phone), u.Status,
		u.MustChangePassword, nullable(u.InternalNotes), u.SecurityFlags,
		nullable(u.UpdatedBy), u.Version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.VersionConflict()
	}
	u.Version++
	return nil
}

func (r *UserRepo) SoftDelete(ctx context.Context, id string, by string) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE users SET deleted_at = now(), updated_by = $2
		WHERE id = $1 AND deleted_at IS NULL`, id, nullable(by))
	return err
}

// List supports allow-listed filters and full-text-ish search on name fields.
func (r *UserRepo) List(ctx context.Context, tenantID string, page ports.ListPage) ([]*domain.User, int, error) {
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
	for field, value := range page.Filters {
		switch field {
		case "email", "phone":
			where = append(where, fmt.Sprintf("%s = $%d", field, i))
			args = append(args, value)
		case "first_name", "last_name":
			where = append(where, fmt.Sprintf("%s ILIKE $%d", field, i))
			args = append(args, value+"%")
		case "status":
			where = append(where, fmt.Sprintf("status = $%d", i))
			args = append(args, value)
		default:
			return nil, 0, fmt.Errorf("filter field not allowed: %s", field)
		}
		i++
	}
	if page.Search != "" {
		where = append(where, fmt.Sprintf("(first_name ILIKE $%d OR last_name ILIKE $%d)", i, i))
		args = append(args, "%"+page.Search+"%")
		i++
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	sortCol := map[string]string{"email": "email", "first_name": "first_name",
		"last_name": "last_name", "status": "status", "created_at": "created_at"}[page.SortBy]
	if sortCol == "" {
		sortCol = "created_at"
	}
	dir := "DESC"
	if page.SortDir == "asc" {
		dir = "ASC"
	}
	// Stable ordering: sort key + id tie-breaker.
	query := fmt.Sprintf(`SELECT %s FROM users WHERE %s ORDER BY %s %s, id %s LIMIT $%d OFFSET $%d`,
		userColumns, whereSQL, sortCol, dir, dir, i, i+1)
	args = append(args, page.PageSize, page.Offset())
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

// Search runs a dynamic filter tree (already validated by the application)
// against the users table, scoped to tenantID plus Row-Level Security.
func (r *UserRepo) Search(ctx context.Context, tenantID string, q domain.SearchQuery) ([]*domain.User, int, error) {
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
	whereSQL, args, next, err := buildWhere(domain.UsersSearchSchema, q, base, baseArgs, len(baseArgs)+1)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := fmt.Sprintf(`SELECT %s FROM users WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d`,
		userColumns, whereSQL, orderClause(domain.UsersSearchSchema, q), next, next+1)
	args = append(args, q.PageSize, q.Offset())
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

// ─── Credentials ────────────────────────────────────────────────────────────

type CredentialRepo struct{}

func NewCredentialRepo() *CredentialRepo { return &CredentialRepo{} }

func (r *CredentialRepo) Create(ctx context.Context, c *domain.Credential) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO user_credentials
		(user_id, tenant_id, password_hash, password_changed_at, failed_login_attempts, locked_until, last_login_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		c.UserID, nullable(c.TenantID), c.PasswordHash, c.PasswordChangedAt,
		c.FailedLoginAttempts, c.LockedUntil, c.LastLoginAt)
	return err
}

func (r *CredentialRepo) GetByUserID(ctx context.Context, userID string) (*domain.Credential, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	c := &domain.Credential{}
	err = tx.QueryRow(ctx, `SELECT user_id, COALESCE(tenant_id::text,''), password_hash,
		password_changed_at, failed_login_attempts, locked_until, last_login_at
		FROM user_credentials WHERE user_id = $1`, userID).
		Scan(&c.UserID, &c.TenantID, &c.PasswordHash, &c.PasswordChangedAt,
			&c.FailedLoginAttempts, &c.LockedUntil, &c.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("Credential")
	}
	return c, err
}

func (r *CredentialRepo) Update(ctx context.Context, c *domain.Credential) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE user_credentials SET password_hash=$2, password_changed_at=$3,
		failed_login_attempts=$4, locked_until=$5, last_login_at=$6 WHERE user_id=$1`,
		c.UserID, c.PasswordHash, c.PasswordChangedAt, c.FailedLoginAttempts, c.LockedUntil, c.LastLoginAt)
	return err
}
