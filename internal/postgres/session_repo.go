package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
)

type SessionRepo struct{}

func NewSessionRepo() *SessionRepo { return &SessionRepo{} }

const sessionColumns = `id, user_id, COALESCE(tenant_id::text,''), family_id, refresh_token_hash,
	status, COALESCE(ip,''), COALESCE(user_agent,''), created_at, last_used_at, expires_at,
	revoked_at, COALESCE(revoke_reason,'')`

func scanSession(row pgx.Row) (*domain.Session, error) {
	s := &domain.Session{}
	err := row.Scan(&s.ID, &s.UserID, &s.TenantID, &s.FamilyID, &s.RefreshTokenHash, &s.Status,
		&s.IP, &s.UserAgent, &s.CreatedAt, &s.LastUsedAt, &s.ExpiresAt, &s.RevokedAt, &s.RevokeReason)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *SessionRepo) Create(ctx context.Context, s *domain.Session) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO sessions (id, user_id, tenant_id, family_id,
		refresh_token_hash, status, ip, user_agent, created_at, last_used_at, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		s.ID, s.UserID, nullable(s.TenantID), s.FamilyID, s.RefreshTokenHash, s.Status,
		nullable(s.IP), nullable(s.UserAgent), s.CreatedAt, s.LastUsedAt, s.ExpiresAt)
	return err
}

func (r *SessionRepo) GetByTokenHash(ctx context.Context, hash string) (*domain.Session, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	s, err := scanSession(tx.QueryRow(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE refresh_token_hash = $1`, hash))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("Session")
	}
	return s, err
}

func (r *SessionRepo) GetByID(ctx context.Context, id string) (*domain.Session, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	s, err := scanSession(tx.QueryRow(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("Session")
	}
	return s, err
}

func (r *SessionRepo) Update(ctx context.Context, s *domain.Session) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE sessions SET status=$2, last_used_at=$3, revoked_at=$4,
		revoke_reason=$5 WHERE id=$1`,
		s.ID, s.Status, s.LastUsedAt, s.RevokedAt, nullable(s.RevokeReason))
	return err
}

func (r *SessionRepo) ListActiveByUser(ctx context.Context, userID string) ([]*domain.Session, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT `+sessionColumns+` FROM sessions
		WHERE user_id = $1 AND status = 'active' AND expires_at > now()
		ORDER BY created_at DESC LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *SessionRepo) RevokeFamily(ctx context.Context, familyID, reason string) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE sessions SET status = CASE WHEN status='rotated' THEN 'reused' ELSE 'revoked' END,
		revoked_at = now(), revoke_reason = $2
		WHERE family_id = $1 AND status IN ('active','rotated')`, familyID, reason)
	return err
}

func (r *SessionRepo) RevokeAllForUser(ctx context.Context, userID, reason string) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE sessions SET status='revoked', revoked_at=now(), revoke_reason=$2
		WHERE user_id = $1 AND status IN ('active','rotated')`, userID, reason)
	return err
}
