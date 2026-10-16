package domain

import (
	"time"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
)

type APIClientStatus string

const (
	APIClientActive    APIClientStatus = "active"
	APIClientSuspended APIClientStatus = "suspended"
	APIClientExpired   APIClientStatus = "expired"
	APIClientRevoked   APIClientStatus = "revoked"
)

// APIClient is a machine-to-machine principal owned by a tenant.
// SecretHash is Argon2id; the plaintext secret is shown exactly once.
type APIClient struct {
	ID              string
	TenantID        string
	Name            string
	Description     string
	ClientID        string
	SecretHash      string
	AllowedIPs      []string
	Scopes          []string
	Status          APIClientStatus
	ExpiresAt       *time.Time
	LastUsedAt      *time.Time
	SecretRotatedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CreatedBy       string
	UpdatedBy       string
	Version         int64
	DeletedAt       *time.Time
}

// CanAuthenticate verifies status, expiry and IP allow-list.
func (c *APIClient) CanAuthenticate(now time.Time, remoteIP string) error {
	switch c.Status {
	case APIClientActive:
	case APIClientRevoked:
		return apperr.Unauthorized("API client has been revoked.")
	case APIClientSuspended:
		return apperr.Unauthorized("API client is suspended.")
	case APIClientExpired:
		return apperr.Unauthorized("API client has expired.")
	default:
		return apperr.Unauthorized("API client is not active.")
	}
	if c.ExpiresAt != nil && now.After(*c.ExpiresAt) {
		return apperr.Unauthorized("API client has expired.")
	}
	if len(c.AllowedIPs) > 0 && remoteIP != "" {
		for _, ip := range c.AllowedIPs {
			if ip == remoteIP {
				return nil
			}
		}
		return apperr.Unauthorized("Request IP is not in the client allow-list.")
	}
	return nil
}

// APIClientPermission is a grant to an API client.
type APIClientPermission struct {
	ID          string
	APIClientID string
	TenantID    string
	Permission  Permission
	GrantedBy   string
	CreatedAt   time.Time
}
