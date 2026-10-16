package httpapi

import (
	"time"

	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
)

// userDTO serializes a user with field-level permission filtering. Protected
// fields the caller may not read are omitted entirely. Secret material
// (password/refresh hashes) never appears here by construction.
func userDTO(v *domain.User, allowedFields []string, isSelf bool) map[string]any {
	allowed := map[string]bool{}
	for _, f := range allowedFields {
		allowed[f] = true
	}
	// A user always sees their own profile fields.
	if isSelf {
		for _, f := range domain.UserProtectedReadFields {
			if f != "internal_notes" && f != "security_flags" {
				allowed[f] = true
			}
		}
	}
	out := map[string]any{
		"id":                   v.ID,
		"tenant_id":            v.TenantID,
		"user_type":            string(v.Type),
		"must_change_password": v.MustChangePassword,
		"created_at":           v.CreatedAt,
		"updated_at":           v.UpdatedAt,
		"version":              v.Version,
	}
	if allowed["email"] {
		out["email"] = v.Email
	}
	if allowed["first_name"] {
		out["first_name"] = v.FirstName
	}
	if allowed["last_name"] {
		out["last_name"] = v.LastName
	}
	if allowed["phone"] {
		out["phone"] = v.Phone
	}
	if allowed["status"] {
		out["status"] = string(v.Status)
	}
	if allowed["internal_notes"] {
		out["internal_notes"] = v.InternalNotes
	}
	if allowed["security_flags"] {
		out["security_flags"] = v.SecurityFlags
	}
	return out
}

type tenantDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Version   int64     `json:"version"`
}

func toTenantDTO(t *domain.Tenant) tenantDTO {
	return tenantDTO{
		ID: t.ID, Name: t.Name, Slug: t.Slug, Status: string(t.Status),
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, Version: t.Version,
	}
}

type roleDTO struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	RoleType    string    `json:"role_type"`
	IsSystem    bool      `json:"is_system"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	Version     int64     `json:"version"`
}

func toRoleDTO(r *domain.Role) roleDTO {
	return roleDTO{
		ID: r.ID, TenantID: r.TenantID, Name: r.Name, Description: r.Description,
		RoleType: string(r.Type), IsSystem: r.IsSystem, Status: r.Status,
		CreatedAt: r.CreatedAt, Version: r.Version,
	}
}

type permissionGrantDTO struct {
	ID           string            `json:"id"`
	Code         string            `json:"code"`
	Service      string            `json:"service"`
	Resource     string            `json:"resource"`
	Action       string            `json:"action"`
	Field        string            `json:"field,omitempty"`
	Scope        string            `json:"scope"`
	Effect       string            `json:"effect"`
	Conditions   map[string]string `json:"conditions,omitempty"`
	DefinitionID string            `json:"permission_id"`
}

func toGrantDTO(id string, p domain.Permission) permissionGrantDTO {
	return permissionGrantDTO{
		ID: id, Code: p.Code(), Service: p.Service, Resource: p.Resource,
		Action: string(p.Action), Field: p.Field, Scope: string(p.Scope),
		Effect: string(p.Effect), Conditions: p.Conditions, DefinitionID: p.DefinitionID,
	}
}

type permissionDefDTO struct {
	ID             string `json:"id"`
	Code           string `json:"code"`
	Service        string `json:"service"`
	Resource       string `json:"resource"`
	Action         string `json:"action"`
	Field          string `json:"field,omitempty"`
	Description    string `json:"description"`
	IsPlatformOnly bool   `json:"is_platform_only"`
}

func toDefDTO(d *domain.PermissionDefinition) permissionDefDTO {
	return permissionDefDTO{
		ID: d.ID, Code: d.Code(), Service: d.Service, Resource: d.Resource,
		Action: string(d.Action), Field: d.Field, Description: d.Description,
		IsPlatformOnly: d.IsPlatformOnly,
	}
}

type apiClientDTO struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	ClientID        string     `json:"client_id"`
	AllowedIPs      []string   `json:"allowed_ips"`
	Scopes          []string   `json:"scopes"`
	Status          string     `json:"status"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
	SecretRotatedAt *time.Time `json:"secret_rotated_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	Version         int64      `json:"version"`
}

func toAPIClientDTO(c *domain.APIClient) apiClientDTO {
	return apiClientDTO{
		ID: c.ID, TenantID: c.TenantID, Name: c.Name, Description: c.Description,
		ClientID: c.ClientID, AllowedIPs: c.AllowedIPs, Scopes: c.Scopes,
		Status: string(c.Status), ExpiresAt: c.ExpiresAt, LastUsedAt: c.LastUsedAt,
		SecretRotatedAt: c.SecretRotatedAt, CreatedAt: c.CreatedAt, Version: c.Version,
	}
}

type sessionDTO struct {
	ID         string    `json:"id"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"user_agent"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Current    bool      `json:"current"`
}

func toSessionDTO(s *domain.Session, currentSessionID string) sessionDTO {
	return sessionDTO{
		ID: s.ID, IP: s.IP, UserAgent: s.UserAgent, CreatedAt: s.CreatedAt,
		LastUsedAt: s.LastUsedAt, ExpiresAt: s.ExpiresAt, Current: s.ID == currentSessionID,
	}
}

type auditLogDTO struct {
	ID            string         `json:"id"`
	OccurredAt    time.Time      `json:"occurred_at"`
	ActorID       string         `json:"actor_id"`
	ActorType     string         `json:"actor_type"`
	TenantID      string         `json:"tenant_id,omitempty"`
	Action        string         `json:"action"`
	Resource      string         `json:"resource"`
	ResourceID    string         `json:"resource_id,omitempty"`
	BeforeState   map[string]any `json:"before_state,omitempty"`
	AfterState    map[string]any `json:"after_state,omitempty"`
	ChangedFields []string       `json:"changed_fields,omitempty"`
	IP            string         `json:"ip,omitempty"`
	UserAgent     string         `json:"user_agent,omitempty"`
	RequestID     string         `json:"request_id,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Result        string         `json:"result"`
	DenialReason  string         `json:"denial_reason,omitempty"`
}

func toAuditDTO(e *domain.AuditLog) auditLogDTO {
	return auditLogDTO{
		ID: e.ID, OccurredAt: e.OccurredAt, ActorID: e.ActorID, ActorType: e.ActorType,
		TenantID: e.TenantID, Action: e.Action, Resource: e.Resource, ResourceID: e.ResourceID,
		BeforeState: e.BeforeState, AfterState: e.AfterState, ChangedFields: e.ChangedFields,
		IP: e.IP, UserAgent: e.UserAgent, RequestID: e.RequestID, CorrelationID: e.CorrelationID,
		Result: string(e.Result), DenialReason: e.DenialReason,
	}
}
