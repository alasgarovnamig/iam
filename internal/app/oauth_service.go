package app

import (
	"context"
	"time"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/auth"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
)

// OAuthService implements the OAuth 2.0 client-credentials grant for API
// clients. Client tokens carry principal_type=api_client and are clearly
// distinguished from user tokens.
type OAuthService struct {
	deps *Deps
}

func NewOAuthService(d *Deps) *OAuthService { return &OAuthService{deps: d} }

type ClientCredentialsCommand struct {
	GrantType    string
	ClientID     string
	ClientSecret string
}

type ClientToken struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope,omitempty"`
}

func (s *OAuthService) Token(ctx context.Context, cmd ClientCredentialsCommand) (*ClientToken, error) {
	if cmd.GrantType != "client_credentials" {
		return nil, apperr.BadRequest("Unsupported grant_type; use client_credentials.")
	}
	if cmd.ClientID == "" || cmd.ClientSecret == "" {
		return nil, apperr.Unauthorized("client_id and client_secret are required.")
	}
	meta := MetaFrom(ctx)

	var client *domain.APIClient
	err := s.deps.Tx.WithTx(ctx, ports.DBContext{Platform: true}, func(txCtx context.Context) error {
		var err error
		client, err = s.deps.APIClients.GetByClientID(txCtx, cmd.ClientID)
		if err != nil {
			// Uniform error to avoid client enumeration.
			return apperr.Unauthorized("Invalid client credentials.")
		}
		now := s.deps.now()
		actor := domain.Principal{ID: client.ID, Kind: domain.KindAPIClient, TenantID: client.TenantID}

		if err := client.CanAuthenticate(now, meta.IP); err != nil {
			_ = s.deps.recordAudit(txCtx, actor, "oauth.token", "api-clients", client.ID, client.TenantID,
				nil, nil, nil, domain.AuditDenied, "client_not_authenticatable")
			return err
		}
		ok, verr := s.deps.Hasher.Verify(cmd.ClientSecret, client.SecretHash)
		if verr != nil || !ok {
			_ = s.deps.recordAudit(txCtx, actor, "oauth.token", "api-clients", client.ID, client.TenantID,
				nil, nil, nil, domain.AuditFailure, "invalid_client_secret")
			return apperr.Unauthorized("Invalid client credentials.")
		}
		// Tenant must be operational for its clients to authenticate.
		tenant, terr := s.deps.Tenants.GetByID(txCtx, client.TenantID)
		if terr != nil || !tenant.IsOperational() {
			return apperr.Unauthorized("Tenant is not active.")
		}
		if err := s.deps.APIClients.TouchLastUsed(txCtx, client.ID, now); err != nil {
			return apperr.Internal(err)
		}
		return s.deps.recordAudit(txCtx, actor, "oauth.token", "api-clients", client.ID, client.TenantID,
			nil, nil, nil, domain.AuditSuccess, "")
	})
	if err != nil {
		return nil, err
	}

	accessToken, expiresAt, err := s.deps.JWT.IssueAccessToken(auth.TokenInput{
		Subject:       client.ID,
		PrincipalType: auth.PrincipalAPIClient,
		TenantID:      client.TenantID,
		ClientID:      client.ClientID,
		Scopes:        client.Scopes,
		JTI:           newID(),
	})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &ClientToken{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(time.Until(expiresAt).Seconds()),
		Scope:       joinScopes(client.Scopes),
	}, nil
}

func joinScopes(scopes []string) string {
	out := ""
	for i, s := range scopes {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}
