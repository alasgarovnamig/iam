package domain

import "time"

type SessionStatus string

const (
	SessionActive  SessionStatus = "active"
	SessionRotated SessionStatus = "rotated"
	SessionRevoked SessionStatus = "revoked"
	SessionReused  SessionStatus = "reused"
)

// Session tracks a refresh-token family member. Refresh tokens are stored
// only as SHA-256 hashes. Rotation creates a new session row in the same
// family; presenting a rotated token again is reuse and revokes the family.
type Session struct {
	ID               string
	UserID           string
	TenantID         string
	FamilyID         string
	RefreshTokenHash string
	Status           SessionStatus
	IP               string
	UserAgent        string
	CreatedAt        time.Time
	LastUsedAt       time.Time
	ExpiresAt        time.Time
	RevokedAt        *time.Time
	RevokeReason     string
}

func (s *Session) IsUsable(now time.Time) bool {
	return s.Status == SessionActive && now.Before(s.ExpiresAt)
}
