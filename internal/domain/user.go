package domain

import (
	"time"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
)

type UserType string

const (
	UserTypePlatform       UserType = "platform_user"
	UserTypeTenant         UserType = "tenant_user"
	UserTypeServiceAccount UserType = "service_account"
)

type UserStatus string

const (
	UserPending   UserStatus = "pending"
	UserActive    UserStatus = "active"
	UserSuspended UserStatus = "suspended"
	UserLocked    UserStatus = "locked"
	UserDisabled  UserStatus = "disabled"
)

// User is the user aggregate root. TenantID is empty for platform users.
type User struct {
	ID                 string
	TenantID           string
	OrganizationUnitID string
	Type               UserType
	Email              string
	FirstName          string
	LastName           string
	Phone              string
	Status             UserStatus
	MustChangePassword bool
	InternalNotes      string
	SecurityFlags      map[string]any
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CreatedBy          string
	UpdatedBy          string
	Version            int64
	DeletedAt          *time.Time
}

func (u *User) IsPlatform() bool { return u.Type == UserTypePlatform }

func (u *User) CanLogin() error {
	switch u.Status {
	case UserActive, UserPending:
		return nil
	case UserLocked:
		return apperr.AccountLocked("Account is locked. Contact an administrator.")
	case UserSuspended:
		return apperr.PermissionDenied("Account is suspended.")
	case UserDisabled:
		return apperr.PermissionDenied("Account is disabled.")
	default:
		return apperr.PermissionDenied("Account is not active.")
	}
}

// validUserTransitions defines the allowed status transitions.
var validUserTransitions = map[UserStatus][]UserStatus{
	UserPending:   {UserActive, UserDisabled},
	UserActive:    {UserSuspended, UserLocked, UserDisabled},
	UserSuspended: {UserActive, UserDisabled},
	UserLocked:    {UserActive, UserDisabled},
	UserDisabled:  {},
}

func (u *User) TransitionTo(next UserStatus) error {
	if u.Status == next {
		return nil
	}
	for _, allowed := range validUserTransitions[u.Status] {
		if allowed == next {
			u.Status = next
			return nil
		}
	}
	return apperr.Conflict("Invalid user status transition from " + string(u.Status) + " to " + string(next))
}

// Credential is the security-sensitive part of a user, stored separately and
// never serialized to API responses.
type Credential struct {
	UserID              string
	TenantID            string
	PasswordHash        string
	PasswordChangedAt   time.Time
	FailedLoginAttempts int
	LockedUntil         *time.Time
	LastLoginAt         *time.Time
}

const (
	MaxFailedLogins = 5
	LockoutDuration = 15 * time.Minute
)

// RegisterFailedLogin increments the counter and reports whether the account
// must transition to locked.
func (c *Credential) RegisterFailedLogin(now time.Time) (lock bool) {
	c.FailedLoginAttempts++
	if c.FailedLoginAttempts >= MaxFailedLogins {
		until := now.Add(LockoutDuration)
		c.LockedUntil = &until
		return true
	}
	return false
}

func (c *Credential) ResetFailures(now time.Time) {
	c.FailedLoginAttempts = 0
	c.LockedUntil = nil
	c.LastLoginAt = &now
}

func (c *Credential) IsTemporarilyLocked(now time.Time) bool {
	return c.LockedUntil != nil && now.Before(*c.LockedUntil)
}
