// Package events defines the versioned event envelope published to Kafka by
// every service through the transactional outbox.
package events

import (
	"encoding/json"
	"time"
)

type Actor struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// Envelope is the wire format for all domain events.
type Envelope struct {
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	EventVersion  int             `json:"event_version"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	TenantID      string          `json:"tenant_id,omitempty"`
	OccurredAt    time.Time       `json:"occurred_at"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	CausationID   string          `json:"causation_id,omitempty"`
	Actor         Actor           `json:"actor"`
	Data          json.RawMessage `json:"data"`
}

// Event types emitted by the user-management service.
const (
	TenantCreatedV1          = "tenant.created.v1"
	TenantUpdatedV1          = "tenant.updated.v1"
	UserCreatedV1            = "user.created.v1"
	UserUpdatedV1            = "user.updated.v1"
	UserStatusChangedV1      = "user.status_changed.v1"
	RoleCreatedV1            = "role.created.v1"
	RoleUpdatedV1            = "role.updated.v1"
	RolePermissionAssignedV1 = "role.permission_assigned.v1"
	RolePermissionRevokedV1  = "role.permission_revoked.v1"
	UserRoleAssignedV1       = "user.role_assigned.v1"
	UserRoleRevokedV1        = "user.role_revoked.v1"
	UserPermissionAssignedV1 = "user.permission_assigned.v1"
	UserPermissionRevokedV1  = "user.permission_revoked.v1"
	APIClientCreatedV1       = "api_client.created.v1"
	APIClientSecretRotatedV1 = "api_client.secret_rotated.v1"
	APIClientRevokedV1       = "api_client.revoked.v1"
)
