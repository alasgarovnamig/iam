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

type AuditRepo struct{}

func NewAuditRepo() *AuditRepo { return &AuditRepo{} }

func (r *AuditRepo) Append(ctx context.Context, e *domain.AuditLog) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	changed := e.ChangedFields
	if changed == nil {
		changed = []string{}
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_logs (id, occurred_at, actor_id, actor_type,
		tenant_id, action, resource, resource_id, before_state, after_state, changed_fields,
		ip, user_agent, request_id, correlation_id, result, denial_reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		e.ID, e.OccurredAt, nullable(e.ActorID), e.ActorType, nullable(e.TenantID),
		e.Action, e.Resource, nullable(e.ResourceID), e.BeforeState, e.AfterState, changed,
		nullable(e.IP), nullable(e.UserAgent), nullable(e.RequestID), nullable(e.CorrelationID),
		e.Result, nullable(e.DenialReason))
	return err
}

const auditColumns = `id, occurred_at, COALESCE(actor_id::text,''), actor_type,
	COALESCE(tenant_id::text,''), action, resource, COALESCE(resource_id,''),
	before_state, after_state, changed_fields, COALESCE(ip,''), COALESCE(user_agent,''),
	COALESCE(request_id,''), COALESCE(correlation_id,''), result, COALESCE(denial_reason,'')`

func scanAudit(row pgx.Row) (*domain.AuditLog, error) {
	e := &domain.AuditLog{}
	err := row.Scan(&e.ID, &e.OccurredAt, &e.ActorID, &e.ActorType, &e.TenantID, &e.Action,
		&e.Resource, &e.ResourceID, &e.BeforeState, &e.AfterState, &e.ChangedFields,
		&e.IP, &e.UserAgent, &e.RequestID, &e.CorrelationID, &e.Result, &e.DenialReason)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (r *AuditRepo) GetByID(ctx context.Context, id string) (*domain.AuditLog, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	e, err := scanAudit(tx.QueryRow(ctx, `SELECT `+auditColumns+` FROM audit_logs WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("Audit log")
	}
	return e, err
}

func (r *AuditRepo) List(ctx context.Context, tenantID string, page ports.ListPage) ([]*domain.AuditLog, int, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, 0, err
	}
	where := []string{"true"}
	args := []any{}
	i := 1
	if tenantID != "" {
		where = append(where, fmt.Sprintf("tenant_id = $%d", i))
		args = append(args, tenantID)
		i++
	}
	for field, value := range page.Filters {
		switch field {
		case "action", "resource", "result":
			where = append(where, fmt.Sprintf("%s = $%d", field, i))
			args = append(args, value)
			i++
		case "actor_id":
			where = append(where, fmt.Sprintf("actor_id = $%d::uuid", i))
			args = append(args, value)
			i++
		}
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := fmt.Sprintf(`SELECT %s FROM audit_logs WHERE %s ORDER BY occurred_at DESC, id DESC LIMIT $%d OFFSET $%d`,
		auditColumns, whereSQL, i, i+1)
	args = append(args, page.PageSize, page.Offset())
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.AuditLog
	for rows.Next() {
		e, err := scanAudit(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// ─── Outbox ─────────────────────────────────────────────────────────────────

type OutboxRepo struct{}

func NewOutboxRepo() *OutboxRepo { return &OutboxRepo{} }

func (r *OutboxRepo) Append(ctx context.Context, ev *domain.OutboxEvent) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	headers := ev.Headers
	if headers == nil {
		headers = map[string]string{}
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events (event_id, event_type, event_version,
		aggregate_type, aggregate_id, tenant_id, payload, headers, status, next_attempt_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		ev.EventID, ev.EventType, ev.EventVersion, ev.AggregateType, ev.AggregateID,
		nullable(ev.TenantID), ev.Payload, headers, ev.Status, ev.NextAttemptAt, ev.CreatedAt)
	return err
}

func (r *OutboxRepo) FetchBatch(ctx context.Context, limit int, now time.Time) ([]*domain.OutboxEvent, error) {
	tx, err := queryer(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id, event_id, event_type, event_version, aggregate_type,
		aggregate_id, COALESCE(tenant_id::text,''), payload, headers, status, attempts,
		next_attempt_at, COALESCE(last_error,''), created_at, published_at
		FROM outbox_events
		WHERE status IN ('pending','failed') AND next_attempt_at <= $1
		ORDER BY id
		LIMIT $2
		FOR UPDATE SKIP LOCKED`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.OutboxEvent
	for rows.Next() {
		ev := &domain.OutboxEvent{}
		if err := rows.Scan(&ev.ID, &ev.EventID, &ev.EventType, &ev.EventVersion, &ev.AggregateType,
			&ev.AggregateID, &ev.TenantID, &ev.Payload, &ev.Headers, &ev.Status, &ev.Attempts,
			&ev.NextAttemptAt, &ev.LastError, &ev.CreatedAt, &ev.PublishedAt); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (r *OutboxRepo) MarkPublished(ctx context.Context, id int64, at time.Time) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE outbox_events SET status='published', published_at=$2 WHERE id=$1`, id, at)
	return err
}

func (r *OutboxRepo) MarkFailed(ctx context.Context, id int64, attempt int, nextAttempt time.Time, lastError string) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE outbox_events SET status='failed', attempts=$2,
		next_attempt_at=$3, last_error=$4 WHERE id=$1`, id, attempt, nextAttempt, lastError)
	return err
}

func (r *OutboxRepo) MarkDead(ctx context.Context, id int64, lastError string) error {
	tx, err := queryer(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE outbox_events SET status='dead', last_error=$2 WHERE id=$1`, id, lastError)
	return err
}
