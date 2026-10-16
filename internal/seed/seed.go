// Package bootstrap contains idempotent startup seeding: the platform
// bootstrap administrator and the OpenFGA relationship tuples that mirror the
// database state.
package seed

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/sr-alasgarov/wallet-gateway/internal/auth"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
)

// SuperAdminRoleID is the fixed id of the Super Admin system role created by
// migration 0008.
const SuperAdminRoleID = "a1000000-0000-4000-8000-000000000001"

// Seeder creates the bootstrap platform administrator. It is idempotent: on
// re-run it verifies the account exists and holds the Super Admin role, and
// never overwrites an existing password.
type Seeder struct {
	Tx          ports.TxManager
	Users       ports.UserRepo
	Credentials ports.CredentialRepo
	Roles       ports.RoleRepo
	FGA         ports.RelationshipStore
	Hasher      *auth.PasswordHasher
	Logger      *slog.Logger
}

// BootstrapAdmin describes the credentials supplied via environment/secret
// manager. The password is never hardcoded in source.
type BootstrapAdmin struct {
	Email    string
	Password string
}

// Run seeds the bootstrap administrator if absent.
func (s *Seeder) Run(ctx context.Context, admin BootstrapAdmin) error {
	if admin.Email == "" || admin.Password == "" {
		return fmt.Errorf("bootstrap admin email and password are required (set BOOTSTRAP_ADMIN_EMAIL / BOOTSTRAP_ADMIN_PASSWORD)")
	}
	if err := auth.ValidatePasswordPolicy(admin.Password); err != nil {
		return fmt.Errorf("bootstrap admin password rejected by policy: %w", err)
	}

	var userID string
	err := s.Tx.WithTx(ctx, ports.DBContext{Platform: true}, func(txCtx context.Context) error {
		// Platform users live in the NULL-tenant space.
		existing, err := s.Users.GetByEmail(txCtx, "", admin.Email)
		if err == nil && existing != nil {
			userID = existing.ID
			s.Logger.Info("bootstrap admin already present, skipping creation",
				"user_id", existing.ID, "email", admin.Email)
			// Make sure the Super Admin role is still attached.
			return s.ensureSuperAdminRole(txCtx, existing.ID)
		}

		hash, err := s.Hasher.Hash(admin.Password)
		if err != nil {
			return fmt.Errorf("hash bootstrap password: %w", err)
		}
		now := time.Now().UTC()
		user := &domain.User{
			ID:        uuid.NewString(),
			Type:      domain.UserTypePlatform,
			Email:     admin.Email,
			FirstName: "Platform",
			LastName:  "Administrator",
			Status:    domain.UserActive,
			// Force a password change on first login.
			MustChangePassword: true,
			SecurityFlags:      map[string]any{"bootstrap": true},
			CreatedAt:          now,
			UpdatedAt:          now,
			Version:            1,
		}
		if err := s.Users.Create(txCtx, user); err != nil {
			return fmt.Errorf("create bootstrap admin: %w", err)
		}
		if err := s.Credentials.Create(txCtx, &domain.Credential{
			UserID: user.ID, PasswordHash: hash, PasswordChangedAt: now,
		}); err != nil {
			return fmt.Errorf("create bootstrap credential: %w", err)
		}
		if err := s.ensureSuperAdminRole(txCtx, user.ID); err != nil {
			return err
		}
		userID = user.ID
		s.Logger.Warn("bootstrap admin created — change this password immediately after first login",
			"user_id", user.ID, "email", admin.Email)
		return nil
	})
	if err != nil {
		return err
	}

	// Relationship tuples: platform membership + super admin.
	tuples := []ports.RelationTuple{
		{User: "user:" + userID, Relation: "member", Object: "platform:platform"},
		{User: "user:" + userID, Relation: "super_admin", Object: "platform:platform"},
		// The Super Admin system role is owned by the platform, so the
		// platform object is the *user* side of role#platform.
		{User: "platform:platform", Relation: "platform", Object: "role:" + SuperAdminRoleID},
		{User: "user:" + userID, Relation: "assignee", Object: "role:" + SuperAdminRoleID},
	}
	if err := s.FGA.Write(ctx, tuples, nil); err != nil {
		// Non-fatal: the API stays up, but platform authorization checks will
		// deny until the tuples exist. Surface it loudly.
		s.Logger.Error("failed to write bootstrap FGA tuples; platform authorization will deny until fixed",
			"error", err)
		return fmt.Errorf("write bootstrap relationships: %w", err)
	}
	return nil
}

func (s *Seeder) ensureSuperAdminRole(ctx context.Context, userID string) error {
	roles, err := s.Roles.ListUserRoles(ctx, userID)
	if err != nil {
		return fmt.Errorf("list bootstrap admin roles: %w", err)
	}
	for _, r := range roles {
		if r.ID == SuperAdminRoleID {
			return nil
		}
	}
	return s.Roles.AssignToUser(ctx, &domain.UserRole{
		ID:        uuid.NewString(),
		UserID:    userID,
		RoleID:    SuperAdminRoleID,
		CreatedAt: time.Now().UTC(),
	})
}
