// Package application contains the use-case layer: command/query services
// orchestrating domain logic through ports. No SQL, HTTP or vendor SDKs here.
package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/auth"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
	"github.com/sr-alasgarov/wallet-gateway/internal/events"
	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
)

// Meta carries per-request metadata for audit and events.
type Meta struct {
	IP            string
	UserAgent     string
	RequestID     string
	CorrelationID string
}

type metaKey struct{}

func WithMeta(ctx context.Context, m Meta) context.Context {
	return context.WithValue(ctx, metaKey{}, m)
}

func MetaFrom(ctx context.Context) Meta {
	if m, ok := ctx.Value(metaKey{}).(Meta); ok {
		return m
	}
	return Meta{}
}

// Deps bundles every port the application services need.
type Deps struct {
	Tx          ports.TxManager
	Tenants     ports.TenantRepo
	Users       ports.UserRepo
	Credentials ports.CredentialRepo
	Sessions    ports.SessionRepo
	Roles       ports.RoleRepo
	Permissions ports.PermissionRepo
	APIClients  ports.APIClientRepo
	Audit       ports.AuditRepo
	Outbox      ports.OutboxRepo

	Cache ports.PermissionCache
	FGA   ports.RelationshipStore
	OPA   ports.PolicyDecider

	Hasher *auth.PasswordHasher
	JWT    *auth.JWTManager

	Logger *slog.Logger
	Now    func() time.Time
}

func (d *Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now().UTC()
}

func newID() string { return uuid.NewString() }

// dbCtxFor returns the RLS context for a principal.
func dbCtxFor(p domain.Principal) ports.DBContext {
	if p.IsPlatform() {
		return ports.DBContext{Platform: true}
	}
	return ports.DBContext{TenantID: p.TenantID}
}

// emitEvent appends a domain event to the transactional outbox (must be
// called inside a transaction).
func (d *Deps) emitEvent(ctx context.Context, actor domain.Principal, eventType, aggregateType, aggregateID, tenantID string, data any) error {
	meta := MetaFrom(ctx)
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	env := events.Envelope{
		EventID:       newID(),
		EventType:     eventType,
		EventVersion:  1,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		TenantID:      tenantID,
		OccurredAt:    d.now(),
		CorrelationID: meta.CorrelationID,
		CausationID:   meta.RequestID,
		Actor:         events.Actor{ID: actor.ID, Type: string(actor.Kind)},
		Data:          payload,
	}
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return d.Outbox.Append(ctx, &domain.OutboxEvent{
		EventID:       env.EventID,
		EventType:     eventType,
		EventVersion:  1,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		TenantID:      tenantID,
		Payload:       body,
		Headers:       map[string]string{"content-type": "application/json"},
		Status:        domain.OutboxPending,
		NextAttemptAt: d.now(),
		CreatedAt:     d.now(),
	})
}

// recordAudit appends an immutable audit entry (inside the transaction when
// part of a mutation).
func (d *Deps) recordAudit(ctx context.Context, actor domain.Principal, action, resource, resourceID, tenantID string,
	before, after map[string]any, changed []string, result domain.AuditResult, denialReason string) error {
	meta := MetaFrom(ctx)
	return d.Audit.Append(ctx, &domain.AuditLog{
		ID:            newID(),
		OccurredAt:    d.now(),
		ActorID:       actor.ID,
		ActorType:     string(actor.Kind),
		TenantID:      tenantID,
		Action:        action,
		Resource:      resource,
		ResourceID:    resourceID,
		BeforeState:   domain.RedactSensitive(before),
		AfterState:    domain.RedactSensitive(after),
		ChangedFields: changed,
		IP:            meta.IP,
		UserAgent:     meta.UserAgent,
		RequestID:     meta.RequestID,
		CorrelationID: meta.CorrelationID,
		Result:        result,
		DenialReason:  denialReason,
	})
}

// auditDenied records an authorization denial in its OWN transaction.
//
// Denials are reported to the caller as errors, which roll the business
// transaction back. An audit written inside that transaction would be rolled
// back with it, silently losing exactly the records security cares about
// (privilege-escalation attempts). Hence a separate, committed transaction.
func (d *Deps) auditDenied(ctx context.Context, actor domain.Principal, action, resource, resourceID, tenantID, reason string) {
	d.auditDeniedWithState(ctx, actor, action, resource, resourceID, tenantID, reason, nil)
}

// auditDeniedWithState is auditDenied with an attempted-change payload
// (e.g. the permission the actor tried to delegate).
func (d *Deps) auditDeniedWithState(ctx context.Context, actor domain.Principal, action, resource, resourceID,
	tenantID, reason string, attempted map[string]any) {
	err := d.Tx.WithTx(ctx, dbCtxFor(actor), func(txCtx context.Context) error {
		return d.recordAudit(txCtx, actor, action, resource, resourceID, tenantID,
			nil, attempted, nil, domain.AuditDenied, reason)
	})
	if err != nil {
		d.Logger.Error("failed to write denial audit", "error", err, "action", action, "reason", reason)
	}
}

// auditDelegationFailure records a rejected delegation attempt when the
// business transaction has already rolled back.
func (d *Deps) auditDelegationFailure(ctx context.Context, actor domain.Principal, err error,
	action, resource, resourceID, tenantID string, attempted map[string]any) {
	if !apperr.Is(err, apperr.CodeDelegationDenied) && !apperr.Is(err, apperr.CodePermissionDenied) {
		return
	}
	d.auditDeniedWithState(ctx, actor, action, resource, resourceID, tenantID,
		apperr.From(err).Detail, attempted)
}
