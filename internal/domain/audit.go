package domain

import "time"

type AuditResult string

const (
	AuditSuccess AuditResult = "success"
	AuditFailure AuditResult = "failure"
	AuditDenied  AuditResult = "denied"
)

// AuditLog is an immutable security event record. Sensitive values (passwords,
// tokens, secrets, hashes) must never be placed in Before/After states —
// the application layer redacts them before constructing the entry.
type AuditLog struct {
	ID            string
	OccurredAt    time.Time
	ActorID       string
	ActorType     string
	TenantID      string
	Action        string
	Resource      string
	ResourceID    string
	BeforeState   map[string]any
	AfterState    map[string]any
	ChangedFields []string
	IP            string
	UserAgent     string
	RequestID     string
	CorrelationID string
	Result        AuditResult
	DenialReason  string
}

// RedactSensitive removes secret material from a state map (defence in depth;
// callers should not put secrets in states at all).
func RedactSensitive(state map[string]any) map[string]any {
	if state == nil {
		return nil
	}
	sensitive := map[string]struct{}{
		"password": {}, "password_hash": {}, "new_password": {}, "current_password": {},
		"secret": {}, "client_secret": {}, "secret_hash": {}, "refresh_token": {},
		"refresh_token_hash": {}, "access_token": {}, "token": {}, "security_flags": {},
	}
	out := make(map[string]any, len(state))
	for k, v := range state {
		if _, bad := sensitive[k]; bad {
			out[k] = "[REDACTED]"
			continue
		}
		out[k] = v
	}
	return out
}
