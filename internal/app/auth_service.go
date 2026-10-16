package app

import (
	"context"
	"strings"
	"time"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/auth"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
	"github.com/sr-alasgarov/wallet-gateway/internal/validation"
)

// AuthService implements user authentication: login, refresh rotation with
// reuse detection, logout, password change and session management.
type AuthService struct {
	deps *Deps
	az   *Authorizer
}

func NewAuthService(d *Deps, az *Authorizer) *AuthService { return &AuthService{deps: d, az: az} }

type LoginCommand struct {
	Email      string
	Password   string
	TenantSlug string // optional disambiguation when the email exists in several tenants
}

type TokenPair struct {
	AccessToken        string       `json:"access_token"`
	RefreshToken       string       `json:"refresh_token"`
	TokenType          string       `json:"token_type"`
	ExpiresAt          time.Time    `json:"expires_at"`
	MustChangePassword bool         `json:"must_change_password"`
	User               *domain.User `json:"-"`
	SessionID          string       `json:"-"`
}

// Login authenticates by email+password, enforcing lockout and audit.
func (s *AuthService) Login(ctx context.Context, cmd LoginCommand) (*TokenPair, error) {
	cmd.Email = strings.TrimSpace(strings.ToLower(cmd.Email))
	if err := validation.Email(cmd.Email); err != nil {
		return nil, apperr.InvalidCredentials()
	}
	if cmd.Password == "" {
		return nil, apperr.InvalidCredentials()
	}

	var pair *TokenPair
	// outcome carries a business rejection (bad password, locked account) out
	// of the transaction. It must NOT be returned from the closure: doing so
	// rolls the transaction back and would discard the very side effects the
	// rejection depends on — the failed-attempt counter, the lock and the
	// audit trail. Infrastructure errors are still returned, and do roll back.
	var outcome error

	// Login runs in platform DB context: the caller has no tenant identity yet.
	err := s.deps.Tx.WithTx(ctx, ports.DBContext{Platform: true}, func(txCtx context.Context) error {
		user, err := s.resolveLoginUser(txCtx, cmd)
		if err != nil {
			return err
		}
		actor := principalForUser(user)

		cred, err := s.deps.Credentials.GetByUserID(txCtx, user.ID)
		if err != nil {
			return apperr.InvalidCredentials().WithCause(err)
		}
		now := s.deps.now()

		if cred.IsTemporarilyLocked(now) || user.Status == domain.UserLocked {
			outcome = apperr.AccountLocked("Account is locked due to failed login attempts. Try again later or contact an administrator.")
			return s.deps.recordAudit(txCtx, actor, "auth.login", "users", user.ID, user.TenantID,
				nil, nil, nil, domain.AuditDenied, "account_locked")
		}
		if err := user.CanLogin(); err != nil {
			outcome = err
			return s.deps.recordAudit(txCtx, actor, "auth.login", "users", user.ID, user.TenantID,
				nil, nil, nil, domain.AuditDenied, "account_not_active")
		}

		ok, verr := s.deps.Hasher.Verify(cmd.Password, cred.PasswordHash)
		if verr != nil || !ok {
			locked := cred.RegisterFailedLogin(now)
			if locked {
				if terr := user.TransitionTo(domain.UserLocked); terr == nil {
					if uerr := s.deps.Users.Update(txCtx, user); uerr != nil {
						return apperr.Internal(uerr)
					}
				}
			}
			if uerr := s.deps.Credentials.Update(txCtx, cred); uerr != nil {
				return apperr.Internal(uerr)
			}
			reason := "invalid_password"
			if locked {
				reason = "invalid_password_account_locked"
			}
			outcome = apperr.InvalidCredentials()
			return s.deps.recordAudit(txCtx, actor, "auth.login", "users", user.ID, user.TenantID,
				nil, nil, nil, domain.AuditFailure, reason)
		}

		cred.ResetFailures(now)
		if err := s.deps.Credentials.Update(txCtx, cred); err != nil {
			return apperr.Internal(err)
		}

		pair, err = s.issueSession(txCtx, user, "", now)
		if err != nil {
			return err
		}
		return s.deps.recordAudit(txCtx, actor, "auth.login", "users", user.ID, user.TenantID,
			nil, nil, nil, domain.AuditSuccess, "")
	})
	if err != nil {
		return nil, err
	}
	if outcome != nil {
		return nil, outcome
	}
	return pair, nil
}

// resolveLoginUser finds the login target: platform user first, then tenant
// users; ambiguous emails require a tenant slug.
func (s *AuthService) resolveLoginUser(ctx context.Context, cmd LoginCommand) (*domain.User, error) {
	if cmd.TenantSlug != "" {
		tenant, err := s.deps.Tenants.GetBySlug(ctx, cmd.TenantSlug)
		if err != nil {
			return nil, apperr.InvalidCredentials().WithCause(err)
		}
		if !tenant.IsOperational() {
			return nil, apperr.PermissionDenied("Tenant is not active.")
		}
		u, err := s.deps.Users.GetByEmail(ctx, tenant.ID, cmd.Email)
		if err != nil {
			return nil, apperr.InvalidCredentials().WithCause(err)
		}
		return u, nil
	}
	candidates, err := s.deps.Users.FindByEmailAny(ctx, cmd.Email)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	switch len(candidates) {
	case 0:
		return nil, apperr.InvalidCredentials()
	case 1:
		return candidates[0], nil
	default:
		// Prefer the platform account when present, otherwise ask for slug.
		for _, c := range candidates {
			if c.IsPlatform() {
				return c, nil
			}
		}
		return nil, apperr.BadRequest("Multiple accounts found for this email; supply tenant_slug.")
	}
}

func principalForUser(u *domain.User) domain.Principal {
	kind := domain.KindTenantUser
	if u.IsPlatform() {
		kind = domain.KindPlatformUser
	} else if u.Type == domain.UserTypeServiceAccount {
		kind = domain.KindServiceAccount
	}
	return domain.Principal{ID: u.ID, Kind: kind, TenantID: u.TenantID, Email: u.Email}
}

// issueSession creates a session row + token pair. familyID empty = new family.
func (s *AuthService) issueSession(ctx context.Context, user *domain.User, familyID string, now time.Time) (*TokenPair, error) {
	refreshToken, err := auth.NewOpaqueToken(32)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if familyID == "" {
		familyID = newID()
	}
	meta := MetaFrom(ctx)
	session := &domain.Session{
		ID:               newID(),
		UserID:           user.ID,
		TenantID:         user.TenantID,
		FamilyID:         familyID,
		RefreshTokenHash: auth.HashToken(refreshToken),
		Status:           domain.SessionActive,
		IP:               meta.IP,
		UserAgent:        meta.UserAgent,
		CreatedAt:        now,
		LastUsedAt:       now,
		ExpiresAt:        now.Add(30 * 24 * time.Hour),
	}
	if err := s.deps.Sessions.Create(ctx, session); err != nil {
		return nil, apperr.Internal(err)
	}
	principalType := auth.PrincipalTenantUser
	if user.IsPlatform() {
		principalType = auth.PrincipalPlatformUser
	} else if user.Type == domain.UserTypeServiceAccount {
		principalType = auth.PrincipalServiceAccount
	}
	access, expiresAt, err := s.deps.JWT.IssueAccessToken(auth.TokenInput{
		Subject:       user.ID,
		PrincipalType: principalType,
		TenantID:      user.TenantID,
		SessionID:     session.ID,
		JTI:           newID(),
	})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &TokenPair{
		AccessToken:        access,
		RefreshToken:       refreshToken,
		TokenType:          "Bearer",
		ExpiresAt:          expiresAt,
		MustChangePassword: user.MustChangePassword,
		User:               user,
		SessionID:          session.ID,
	}, nil
}

// Refresh rotates a refresh token. Presenting an already-rotated/revoked
// token is treated as theft: the whole family is revoked.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	if refreshToken == "" {
		return nil, apperr.Unauthorized("Missing refresh token.")
	}
	hash := auth.HashToken(refreshToken)
	var pair *TokenPair
	// See Login: a business rejection travels in `outcome` so the transaction
	// still commits. For reuse detection this is critical — returning the
	// error directly would roll back the family revocation, leaving the
	// stolen token chain usable.
	var outcome error

	err := s.deps.Tx.WithTx(ctx, ports.DBContext{Platform: true}, func(txCtx context.Context) error {
		session, err := s.deps.Sessions.GetByTokenHash(txCtx, hash)
		if err != nil {
			return apperr.Unauthorized("Invalid refresh token.")
		}
		now := s.deps.now()
		user, err := s.deps.Users.GetByID(txCtx, session.UserID)
		if err != nil {
			return apperr.Unauthorized("Invalid refresh token.")
		}
		actor := principalForUser(user)

		if session.Status != domain.SessionActive {
			// Reuse detected → revoke the entire family (must be committed).
			if err := s.deps.Sessions.RevokeFamily(txCtx, session.FamilyID, "refresh_token_reuse"); err != nil {
				return apperr.Internal(err)
			}
			outcome = apperr.TokenReuse()
			return s.deps.recordAudit(txCtx, actor, "auth.refresh", "sessions", session.ID, session.TenantID,
				nil, nil, nil, domain.AuditDenied, "refresh_token_reuse_detected")
		}
		if !session.IsUsable(now) {
			outcome = apperr.TokenExpired()
			return nil
		}
		if err := user.CanLogin(); err != nil {
			outcome = err
			return nil
		}

		session.Status = domain.SessionRotated
		session.LastUsedAt = now
		if err := s.deps.Sessions.Update(txCtx, session); err != nil {
			return apperr.Internal(err)
		}
		pair, err = s.issueSession(txCtx, user, session.FamilyID, now)
		if err != nil {
			return err
		}
		return s.deps.recordAudit(txCtx, actor, "auth.refresh", "sessions", session.ID, session.TenantID,
			nil, nil, nil, domain.AuditSuccess, "")
	})
	if err != nil {
		return nil, err
	}
	if outcome != nil {
		return nil, outcome
	}
	return pair, nil
}

// Logout revokes the session belonging to the presented refresh token.
func (s *AuthService) Logout(ctx context.Context, p domain.Principal, refreshToken string) error {
	return s.deps.Tx.WithTx(ctx, ports.DBContext{Platform: true}, func(txCtx context.Context) error {
		if refreshToken != "" {
			if session, err := s.deps.Sessions.GetByTokenHash(txCtx, auth.HashToken(refreshToken)); err == nil {
				if err := s.deps.Sessions.RevokeFamily(txCtx, session.FamilyID, "logout"); err != nil {
					return apperr.Internal(err)
				}
			}
		} else if p.SessionID != "" {
			if session, err := s.deps.Sessions.GetByID(txCtx, p.SessionID); err == nil {
				if err := s.deps.Sessions.RevokeFamily(txCtx, session.FamilyID, "logout"); err != nil {
					return apperr.Internal(err)
				}
			}
		}
		return s.deps.recordAudit(txCtx, p, "auth.logout", "sessions", p.SessionID, p.TenantID,
			nil, nil, nil, domain.AuditSuccess, "")
	})
}

// LogoutAll revokes every session of the principal.
func (s *AuthService) LogoutAll(ctx context.Context, p domain.Principal) error {
	return s.deps.Tx.WithTx(ctx, ports.DBContext{Platform: true}, func(txCtx context.Context) error {
		if err := s.deps.Sessions.RevokeAllForUser(txCtx, p.ID, "logout_all"); err != nil {
			return apperr.Internal(err)
		}
		return s.deps.recordAudit(txCtx, p, "auth.logout_all", "sessions", "", p.TenantID,
			nil, nil, nil, domain.AuditSuccess, "")
	})
}

type ChangePasswordCommand struct {
	CurrentPassword string
	NewPassword     string
}

// ChangePassword verifies the current password, applies the policy, rotates
// the hash and revokes all other sessions.
func (s *AuthService) ChangePassword(ctx context.Context, p domain.Principal, cmd ChangePasswordCommand) error {
	if err := auth.ValidatePasswordPolicy(cmd.NewPassword); err != nil {
		return apperr.Validation(err.Error())
	}
	if cmd.NewPassword == cmd.CurrentPassword {
		return apperr.Validation("New password must differ from the current password.")
	}
	// As in Login: a wrong current password is a business rejection whose
	// audit record must survive, so it leaves the closure via `outcome`.
	var outcome error
	err := s.deps.Tx.WithTx(ctx, ports.DBContext{Platform: true}, func(txCtx context.Context) error {
		cred, err := s.deps.Credentials.GetByUserID(txCtx, p.ID)
		if err != nil {
			return apperr.Internal(err)
		}
		ok, verr := s.deps.Hasher.Verify(cmd.CurrentPassword, cred.PasswordHash)
		if verr != nil || !ok {
			outcome = apperr.InvalidCredentials()
			return s.deps.recordAudit(txCtx, p, "auth.change_password", "users", p.ID, p.TenantID,
				nil, nil, nil, domain.AuditFailure, "invalid_current_password")
		}
		newHash, err := s.deps.Hasher.Hash(cmd.NewPassword)
		if err != nil {
			return apperr.Internal(err)
		}
		cred.PasswordHash = newHash
		cred.PasswordChangedAt = s.deps.now()
		if err := s.deps.Credentials.Update(txCtx, cred); err != nil {
			return apperr.Internal(err)
		}
		user, err := s.deps.Users.GetByID(txCtx, p.ID)
		if err != nil {
			return apperr.Internal(err)
		}
		if user.MustChangePassword {
			user.MustChangePassword = false
			if err := s.deps.Users.Update(txCtx, user); err != nil {
				return apperr.Internal(err)
			}
		}
		// Keep only the current session alive.
		sessions, err := s.deps.Sessions.ListActiveByUser(txCtx, p.ID)
		if err != nil {
			return apperr.Internal(err)
		}
		for _, sess := range sessions {
			if sess.ID == p.SessionID {
				continue
			}
			if err := s.deps.Sessions.RevokeFamily(txCtx, sess.FamilyID, "password_changed"); err != nil {
				return apperr.Internal(err)
			}
		}
		return s.deps.recordAudit(txCtx, p, "auth.change_password", "users", p.ID, p.TenantID,
			nil, nil, []string{"password"}, domain.AuditSuccess, "")
	})
	if err != nil {
		return err
	}
	return outcome
}

// Me returns the principal's own user record (no field filtering: it is self).
func (s *AuthService) Me(ctx context.Context, p domain.Principal) (*domain.User, []string, error) {
	var user *domain.User
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		user, err = s.deps.Users.GetByID(txCtx, p.ID)
		return err
	})
	if err != nil {
		return nil, nil, apperr.NotFound("User")
	}
	permSet, err := s.az.EffectivePermissions(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	codes := make([]string, 0, len(permSet.All()))
	for _, perm := range permSet.All() {
		if perm.Effect == domain.EffectAllow {
			codes = append(codes, perm.Code()+":"+string(perm.Scope))
		}
	}
	return user, codes, nil
}

// Sessions lists the principal's active sessions.
func (s *AuthService) Sessions(ctx context.Context, p domain.Principal) ([]*domain.Session, error) {
	var out []*domain.Session
	err := s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		var err error
		out, err = s.deps.Sessions.ListActiveByUser(txCtx, p.ID)
		return err
	})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return out, nil
}

// RevokeSession revokes one of the principal's own sessions.
func (s *AuthService) RevokeSession(ctx context.Context, p domain.Principal, sessionID string) error {
	return s.deps.Tx.WithTx(ctx, dbCtxFor(p), func(txCtx context.Context) error {
		session, err := s.deps.Sessions.GetByID(txCtx, sessionID)
		if err != nil {
			return apperr.NotFound("Session")
		}
		if session.UserID != p.ID {
			return apperr.PermissionDenied("You may only revoke your own sessions.")
		}
		if err := s.deps.Sessions.RevokeFamily(txCtx, session.FamilyID, "revoked_by_user"); err != nil {
			return apperr.Internal(err)
		}
		return s.deps.recordAudit(txCtx, p, "auth.session_revoked", "sessions", sessionID, p.TenantID,
			nil, nil, nil, domain.AuditSuccess, "")
	})
}
