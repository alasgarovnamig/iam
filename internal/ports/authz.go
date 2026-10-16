package ports

import (
	"context"
	"time"

	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
)

// RelationshipStore is the OpenFGA port: relationship/ownership graph.
type RelationshipStore interface {
	// Check returns whether `user` has `relation` on `object` (e.g.
	// Check(ctx, "user:123", "admin", "tenant:456")).
	Check(ctx context.Context, user, relation, object string) (bool, error)
	// Write adds tuples; delete removes them. Both are idempotent-safe.
	Write(ctx context.Context, writes []RelationTuple, deletes []RelationTuple) error
	Healthy(ctx context.Context) error
}

type RelationTuple struct {
	User     string
	Relation string
	Object   string
}

// PolicyInput is the request context sent to OPA.
type PolicyInput struct {
	Principal struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		TenantID string `json:"tenant_id,omitempty"`
	} `json:"principal"`
	Request struct {
		Method   string `json:"method"`
		Path     string `json:"path"`
		Action   string `json:"action"`
		Resource string `json:"resource"`
	} `json:"request"`
	Target struct {
		TenantID   string `json:"tenant_id,omitempty"`
		ResourceID string `json:"resource_id,omitempty"`
	} `json:"target"`
	// Effective permissions rendered as objects for the policy.
	Permissions   []PolicyPermission `json:"permissions"`
	Relationships map[string]bool    `json:"relationships"`
	Context       struct {
		IP   string `json:"ip"`
		Time string `json:"time"`
	} `json:"context"`
}

type PolicyPermission struct {
	Code   string            `json:"code"`
	Scope  string            `json:"scope"`
	Effect string            `json:"effect"`
	Conds  map[string]string `json:"conditions,omitempty"`
}

// PolicyDecision is OPA's answer.
type PolicyDecision struct {
	Allow         bool              `json:"allow"`
	Reason        string            `json:"reason"`
	AllowedFields []string          `json:"allowed_fields"`
	DeniedFields  []string          `json:"denied_fields"`
	Filters       map[string]string `json:"filters"`
}

// PolicyDecider is the OPA port. Implementations must FAIL CLOSED: any error
// results in a deny at the caller.
type PolicyDecider interface {
	Decide(ctx context.Context, in PolicyInput) (*PolicyDecision, error)
	Healthy(ctx context.Context) error
}

// PermissionCache caches effective permission sets per principal.
// Implementations must treat cache errors as misses (fail closed to DB, not
// open) and support tenant-wide invalidation.
type PermissionCache interface {
	Get(ctx context.Context, tenantID, principalID string) ([]domain.Permission, bool)
	Set(ctx context.Context, tenantID, principalID string, perms []domain.Permission, ttl time.Duration)
	InvalidatePrincipal(ctx context.Context, tenantID, principalID string)
	InvalidateTenant(ctx context.Context, tenantID string)
}

// RateLimiter is the fixed/sliding window limiter port.
type RateLimiter interface {
	// Allow returns false when the key exceeded limit within window.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}
