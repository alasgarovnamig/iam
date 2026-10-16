package app

import (
	"context"
	"strings"
	"time"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/auth"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
	"github.com/sr-alasgarov/wallet-gateway/internal/events"
	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
	"github.com/sr-alasgarov/wallet-gateway/internal/validation"
)

// APIClientService manages machine-to-machine clients. Secrets are Argon2id
// hashed and returned in plaintext exactly once (creation and rotation).
type APIClientService struct {
	deps *Deps
	az   *Authorizer
}

func NewAPIClientService(d *Deps, az *Authorizer) *APIClientService {
	return &APIClientService{deps: d, az: az}
}

type CreateAPIClientCommand struct {
	TenantID    string
	Name        string
	Description string
	AllowedIPs  []string
	Scopes      []string
	ExpiresAt   *time.Time
}

// CreatedAPIClient carries the one-time plaintext secret.
type CreatedAPIClient struct {
	Client       *domain.APIClient
	ClientSecret string
}

func (s *APIClientService) Create(ctx context.Context, p domain.Principal, cmd CreateAPIClientCommand) (*CreatedAPIClient, error) {
	targetTenant := cmd.TenantID
	if !p.IsPlatform() {
		targetTenant = p.TenantID
	}
	if targetTenant == "" {
		return nil, apperr.Validation("tenant_id is required for API clients.")
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "api-clients", Action: domain.ActionCreate, Method: "POST", Path: "/api/v1/api-clients",
		TargetTenantID: targetTenant,
	}); err != nil {
		return nil, err
	}
	cmd.Name = strings.TrimSpace(cmd.Name)
	if err := validation.Required("name", cmd.Name); err != nil {
		return nil, apperr.Validation(err.Error())
	}

	clientID, err := auth.NewOpaqueToken(12)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	clientID = "wg_" + clientID
	secret, err := auth.NewOpaqueToken(32)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	secretHash, err := s.deps.Hasher.Hash(secret)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	now := s.deps.now()
	client := &domain.APIClient{
		ID:          newID(),
		TenantID:    targetTenant,
		Name:        cmd.Name,
		Description: strings.TrimSpace(cmd.Description),
		ClientID:    clientID,
		SecretHash:  secretHash,
		AllowedIPs:  cmd.AllowedIPs,
		Scopes:      cmd.Scopes,
		Status:      domain.APIClientActive,
		ExpiresAt:   cmd.ExpiresAt,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   p.ID,
		UpdatedBy:   p.ID,
		Version:     1,
	}
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		if err := s.deps.APIClients.Create(txCtx, client); err != nil {
			return err
		}
		if err := s.deps.recordAudit(txCtx, p, "api_client.created", "api-clients", client.ID, targetTenant,
			nil, map[string]any{"name": client.Name, "client_id": client.ClientID},
			[]string{"name", "client_id"}, domain.AuditSuccess, ""); err != nil {
			return err
		}
		return s.deps.emitEvent(txCtx, p, events.APIClientCreatedV1, "api_client", client.ID, targetTenant,
			map[string]any{"name": client.Name, "client_id": client.ClientID})
	})
	if err != nil {
		return nil, apperr.From(err)
	}
	if err := s.deps.FGA.Write(ctx, []ports.RelationTuple{
		{User: "api_client:" + client.ID, Relation: "member", Object: "tenant:" + targetTenant},
	}, nil); err != nil {
		s.deps.Logger.Error("failed to write api client FGA tuple", "client_id", client.ID, "error", err)
	}
	return &CreatedAPIClient{Client: client, ClientSecret: secret}, nil
}

func (s *APIClientService) Get(ctx context.Context, p domain.Principal, id string) (*domain.APIClient, []*domain.APIClientPermission, error) {
	if err := validation.UUID(id); err != nil {
		return nil, nil, apperr.BadRequest("Invalid API client id.")
	}
	var client *domain.APIClient
	var perms []*domain.APIClientPermission
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		client, err = s.deps.APIClients.GetByID(txCtx, id)
		if err != nil {
			return err
		}
		perms, err = s.deps.APIClients.ListPermissions(txCtx, id)
		return err
	})
	if err != nil {
		return nil, nil, apperr.NotFound("API client")
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "api-clients", Action: domain.ActionRead, Method: "GET", Path: "/api/v1/api-clients/" + id,
		TargetTenantID: client.TenantID,
	}); err != nil {
		return nil, nil, err
	}
	return client, perms, nil
}

func (s *APIClientService) List(ctx context.Context, p domain.Principal, tenantID string, page ports.ListPage) ([]*domain.APIClient, int, error) {
	targetTenant := tenantID
	if !p.IsPlatform() {
		targetTenant = p.TenantID
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "api-clients", Action: domain.ActionList, Method: "GET", Path: "/api/v1/api-clients",
		TargetTenantID: targetTenant,
	}); err != nil {
		return nil, 0, err
	}
	sanitizeListPage(&page, map[string]bool{"name": true, "created_at": true, "status": true},
		map[string]bool{"status": true})
	var out []*domain.APIClient
	var total int
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		out, total, err = s.deps.APIClients.List(txCtx, targetTenant, page)
		return err
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	return out, total, nil
}

// Search runs a dynamic filter tree over API clients. Requires
// api-clients.search plus api-clients.search.<field> for every field touched.
func (s *APIClientService) Search(ctx context.Context, p domain.Principal, tenantID string, cmd SearchCommand) ([]*domain.APIClient, int, error) {
	targetTenant := tenantID
	if !p.IsPlatform() {
		targetTenant = p.TenantID
	}
	dec, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "api-clients", Action: domain.ActionSearch, Method: "POST", Path: "/api/v1/api-clients/search",
		TargetTenantID: targetTenant,
	})
	if err != nil {
		return nil, 0, err
	}
	q, err := authorizeSearch(dec, domain.APIClientsSearchSchema, cmd)
	if err != nil {
		return nil, 0, err
	}
	var out []*domain.APIClient
	var total int
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		out, total, err = s.deps.APIClients.Search(txCtx, targetTenant, q)
		return err
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	return out, total, nil
}

type UpdateAPIClientCommand struct {
	Name        *string
	Description *string
	AllowedIPs  *[]string
	Status      *string
	Version     int64
}

func (s *APIClientService) Update(ctx context.Context, p domain.Principal, id string, cmd UpdateAPIClientCommand) (*domain.APIClient, error) {
	if err := validation.UUID(id); err != nil {
		return nil, apperr.BadRequest("Invalid API client id.")
	}
	var client *domain.APIClient
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		client, err = s.deps.APIClients.GetByID(txCtx, id)
		return err
	})
	if err != nil {
		return nil, apperr.NotFound("API client")
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "api-clients", Action: domain.ActionUpdate, Method: "PATCH", Path: "/api/v1/api-clients/" + id,
		TargetTenantID: client.TenantID,
	}); err != nil {
		return nil, err
	}
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		client, err = s.deps.APIClients.GetByID(txCtx, id)
		if err != nil {
			return apperr.NotFound("API client")
		}
		if cmd.Version != 0 && cmd.Version != client.Version {
			return apperr.VersionConflict()
		}
		var changed []string
		before := map[string]any{"name": client.Name, "status": string(client.Status)}
		if cmd.Name != nil && *cmd.Name != client.Name {
			client.Name = strings.TrimSpace(*cmd.Name)
			changed = append(changed, "name")
		}
		if cmd.Description != nil && *cmd.Description != client.Description {
			client.Description = strings.TrimSpace(*cmd.Description)
			changed = append(changed, "description")
		}
		if cmd.AllowedIPs != nil {
			client.AllowedIPs = *cmd.AllowedIPs
			changed = append(changed, "allowed_ips")
		}
		if cmd.Status != nil && *cmd.Status != string(client.Status) {
			if err := validation.OneOf("status", *cmd.Status, "active", "suspended"); err != nil {
				return apperr.Validation(err.Error())
			}
			client.Status = domain.APIClientStatus(*cmd.Status)
			changed = append(changed, "status")
		}
		if len(changed) == 0 {
			return nil
		}
		client.UpdatedBy = p.ID
		if err := s.deps.APIClients.Update(txCtx, client); err != nil {
			return err
		}
		return s.deps.recordAudit(txCtx, p, "api_client.updated", "api-clients", id, client.TenantID,
			before, map[string]any{"name": client.Name, "status": string(client.Status)},
			changed, domain.AuditSuccess, "")
	})
	if err != nil {
		return nil, apperr.From(err)
	}
	s.az.InvalidatePermissions(ctx, client.TenantID, client.ID)
	return client, nil
}

// Delete revokes the client permanently.
func (s *APIClientService) Delete(ctx context.Context, p domain.Principal, id string) error {
	if err := validation.UUID(id); err != nil {
		return apperr.BadRequest("Invalid API client id.")
	}
	var client *domain.APIClient
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		client, err = s.deps.APIClients.GetByID(txCtx, id)
		return err
	})
	if err != nil {
		return apperr.NotFound("API client")
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "api-clients", Action: domain.ActionDelete, Method: "DELETE", Path: "/api/v1/api-clients/" + id,
		TargetTenantID: client.TenantID,
	}); err != nil {
		return err
	}
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		client.Status = domain.APIClientRevoked
		client.UpdatedBy = p.ID
		if err := s.deps.APIClients.Update(txCtx, client); err != nil {
			return err
		}
		if err := s.deps.APIClients.SoftDelete(txCtx, id, p.ID); err != nil {
			return err
		}
		if err := s.deps.recordAudit(txCtx, p, "api_client.revoked", "api-clients", id, client.TenantID,
			nil, nil, []string{"status"}, domain.AuditSuccess, ""); err != nil {
			return err
		}
		return s.deps.emitEvent(txCtx, p, events.APIClientRevokedV1, "api_client", id, client.TenantID,
			map[string]any{"client_id": client.ClientID})
	})
	if err != nil {
		return apperr.From(err)
	}
	s.az.InvalidatePermissions(ctx, client.TenantID, client.ID)
	return nil
}

// RotateSecret replaces the secret; plaintext returned once.
func (s *APIClientService) RotateSecret(ctx context.Context, p domain.Principal, id string) (*CreatedAPIClient, error) {
	if err := validation.UUID(id); err != nil {
		return nil, apperr.BadRequest("Invalid API client id.")
	}
	var client *domain.APIClient
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		client, err = s.deps.APIClients.GetByID(txCtx, id)
		return err
	})
	if err != nil {
		return nil, apperr.NotFound("API client")
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "api-clients", Action: domain.ActionRotate, Method: "POST",
		Path: "/api/v1/api-clients/" + id + "/rotate-secret", TargetTenantID: client.TenantID,
	}); err != nil {
		return nil, err
	}
	secret, err := auth.NewOpaqueToken(32)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	secretHash, err := s.deps.Hasher.Hash(secret)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		client, err = s.deps.APIClients.GetByID(txCtx, id)
		if err != nil {
			return apperr.NotFound("API client")
		}
		now := s.deps.now()
		client.SecretHash = secretHash
		client.SecretRotatedAt = &now
		client.UpdatedBy = p.ID
		if err := s.deps.APIClients.Update(txCtx, client); err != nil {
			return err
		}
		if err := s.deps.recordAudit(txCtx, p, "api_client.secret_rotated", "api-clients", id, client.TenantID,
			nil, nil, []string{"secret"}, domain.AuditSuccess, ""); err != nil {
			return err
		}
		return s.deps.emitEvent(txCtx, p, events.APIClientSecretRotatedV1, "api_client", id, client.TenantID,
			map[string]any{"client_id": client.ClientID})
	})
	if err != nil {
		return nil, apperr.From(err)
	}
	return &CreatedAPIClient{Client: client, ClientSecret: secret}, nil
}

type ClientGrantCommand struct {
	PermissionID string
	Scope        string
	Effect       string
	Conditions   map[string]string
}

// GrantPermission applies the delegation subset rule to API client grants.
func (s *APIClientService) GrantPermission(ctx context.Context, p domain.Principal, clientID string, cmd ClientGrantCommand) (*domain.APIClientPermission, error) {
	if err := validation.UUID(clientID); err != nil {
		return nil, apperr.BadRequest("Invalid API client id.")
	}
	var client *domain.APIClient
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		client, err = s.deps.APIClients.GetByID(txCtx, clientID)
		return err
	})
	if err != nil {
		return nil, apperr.NotFound("API client")
	}
	dec, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "api-clients", Action: domain.ActionAssign, Method: "POST",
		Path: "/api/v1/api-clients/" + clientID + "/permissions", TargetTenantID: client.TenantID,
	})
	if err != nil {
		return nil, err
	}
	var grant *domain.APIClientPermission
	attempted := map[string]any{"permission_id": cmd.PermissionID, "scope": cmd.Scope, "effect": cmd.Effect}
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		def, err := s.deps.Permissions.GetDefinition(txCtx, cmd.PermissionID)
		if err != nil {
			return apperr.NotFound("Permission definition")
		}
		perm, err := buildGrant(def, cmd.Scope, cmd.Effect, cmd.Conditions)
		if err != nil {
			return err
		}
		attempted["permission"] = perm.Code()
		if def.IsPlatformOnly {
			return apperr.DelegationDenied("Platform-only permissions cannot be granted to API clients.")
		}
		if perm.Scope == domain.ScopeAll {
			return apperr.DelegationDenied("API clients cannot receive scope=all.")
		}
		if err := dec.PermSet.CanDelegateAll([]domain.Permission{perm}); err != nil {
			return apperr.DelegationDenied(err.Error())
		}
		grant = &domain.APIClientPermission{
			ID: newID(), APIClientID: clientID, TenantID: client.TenantID, Permission: perm,
			GrantedBy: p.ID, CreatedAt: s.deps.now(),
		}
		if err := s.deps.APIClients.GrantPermission(txCtx, grant); err != nil {
			return err
		}
		return s.deps.recordAudit(txCtx, p, "api_client.permission_assigned", "api-clients", clientID, client.TenantID,
			nil, map[string]any{"permission": perm.Code(), "scope": string(perm.Scope)}, nil, domain.AuditSuccess, "")
	})
	if err != nil {
		s.deps.auditDelegationFailure(ctx, p, err, "api_client.permission_assign", "api-clients",
			clientID, client.TenantID, attempted)
		return nil, apperr.From(err)
	}
	s.az.InvalidatePermissions(ctx, client.TenantID, client.ID)
	return grant, nil
}

func (s *APIClientService) RevokePermission(ctx context.Context, p domain.Principal, clientID, permissionRowID string) error {
	if err := validation.UUID(clientID); err != nil {
		return apperr.BadRequest("Invalid API client id.")
	}
	var client *domain.APIClient
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		client, err = s.deps.APIClients.GetByID(txCtx, clientID)
		return err
	})
	if err != nil {
		return apperr.NotFound("API client")
	}
	if _, err := s.az.MustAllow(ctx, p, AuthzRequest{
		Resource: "api-clients", Action: domain.ActionRevoke, Method: "DELETE",
		Path:           "/api/v1/api-clients/" + clientID + "/permissions/" + permissionRowID,
		TargetTenantID: client.TenantID,
	}); err != nil {
		return err
	}
	err = s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		if err := s.deps.APIClients.RevokePermission(txCtx, clientID, permissionRowID); err != nil {
			return err
		}
		return s.deps.recordAudit(txCtx, p, "api_client.permission_revoked", "api-clients", clientID, client.TenantID,
			nil, nil, nil, domain.AuditSuccess, "")
	})
	if err != nil {
		return apperr.From(err)
	}
	s.az.InvalidatePermissions(ctx, client.TenantID, client.ID)
	return nil
}
