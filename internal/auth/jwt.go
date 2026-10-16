package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// PrincipalType distinguishes token subjects.
type PrincipalType string

const (
	PrincipalPlatformUser   PrincipalType = "platform_user"
	PrincipalTenantUser     PrincipalType = "tenant_user"
	PrincipalAPIClient      PrincipalType = "api_client"
	PrincipalServiceAccount PrincipalType = "service_account"
	PrincipalSystemService  PrincipalType = "system_service"
)

// Claims is the JWT claim set used across the platform. Authorization data is
// deliberately NOT embedded: permissions are resolved at request time.
type Claims struct {
	PrincipalType PrincipalType `json:"principal_type"`
	TenantID      string        `json:"tenant_id,omitempty"`
	SessionID     string        `json:"session_id,omitempty"`
	ClientID      string        `json:"client_id,omitempty"`
	Scopes        []string      `json:"scopes,omitempty"`
	jwt.RegisteredClaims
}

type JWTConfig struct {
	SigningKey     []byte
	Issuer         string
	Audience       string
	AccessTokenTTL time.Duration
	ClientTokenTTL time.Duration
}

type JWTManager struct {
	cfg JWTConfig
	now func() time.Time
}

func NewJWTManager(cfg JWTConfig) *JWTManager {
	return &JWTManager{cfg: cfg, now: time.Now}
}

// WithClock overrides the time source (tests).
func (m *JWTManager) WithClock(now func() time.Time) *JWTManager {
	clone := *m
	clone.now = now
	return &clone
}

type TokenInput struct {
	Subject       string
	PrincipalType PrincipalType
	TenantID      string
	SessionID     string
	ClientID      string
	Scopes        []string
	JTI           string
}

// IssueAccessToken creates a signed access token; TTL depends on principal type.
func (m *JWTManager) IssueAccessToken(in TokenInput) (token string, expiresAt time.Time, err error) {
	ttl := m.cfg.AccessTokenTTL
	if in.PrincipalType == PrincipalAPIClient {
		ttl = m.cfg.ClientTokenTTL
	}
	now := m.now()
	expiresAt = now.Add(ttl)
	claims := Claims{
		PrincipalType: in.PrincipalType,
		TenantID:      in.TenantID,
		SessionID:     in.SessionID,
		ClientID:      in.ClientID,
		Scopes:        in.Scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   in.Subject,
			Issuer:    m.cfg.Issuer,
			Audience:  jwt.ClaimStrings{m.cfg.Audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        in.JTI,
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.cfg.SigningKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, expiresAt, nil
}

// Parse validates signature, algorithm, issuer, audience and time claims.
func (m *JWTManager) Parse(tokenString string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return m.cfg.SigningKey, nil
	},
		jwt.WithIssuer(m.cfg.Issuer),
		jwt.WithAudience(m.cfg.Audience),
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil {
		return nil, err
	}
	if claims.Subject == "" || claims.PrincipalType == "" {
		return nil, fmt.Errorf("token missing required claims")
	}
	return claims, nil
}
