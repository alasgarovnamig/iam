package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/auth"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
)

// These tests pin down the transaction semantics of authentication.
//
// Login and Refresh must persist their security side effects (failed-attempt
// counters, account locks, refresh-family revocation, audit records) even
// though they answer the caller with an error. An earlier implementation
// returned those errors from inside the transaction closure, which rolled the
// side effects back: brute-force protection and refresh-token reuse detection
// silently did nothing. The fake TxManager below reproduces that semantics
// exactly — a closure error discards all writes — so the tests fail loudly if
// the pattern regresses.

// ─── Fakes ──────────────────────────────────────────────────────────────────

// fakeTx models a real transaction: writes performed inside the closure are
// staged and only become visible if the closure returns nil.
type fakeTx struct {
	store    *fakeStore
	commits  int
	rollback int
}

func (f *fakeTx) WithTx(ctx context.Context, dbc ports.DBContext, fn func(ctx context.Context) error) error {
	// Nested transactions reuse the outer one (as the pgx manager does).
	if f.store.inTx {
		return fn(ctx)
	}
	f.store.begin()
	f.store.inTx = true
	err := fn(ctx)
	f.store.inTx = false
	if err != nil {
		f.store.rollback()
		f.rollback++
		return err
	}
	f.store.commit()
	f.commits++
	return nil
}

// fakeStore holds committed state plus the pending writes of a transaction.
type fakeStore struct {
	inTx bool

	users       map[string]*domain.User
	credentials map[string]*domain.Credential
	sessions    map[string]*domain.Session
	audits      []*domain.AuditLog

	// snapshots taken at begin(), restored on rollback()
	snapUsers   map[string]domain.User
	snapCreds   map[string]domain.Credential
	snapSess    map[string]domain.Session
	snapAuditsN int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		users:       map[string]*domain.User{},
		credentials: map[string]*domain.Credential{},
		sessions:    map[string]*domain.Session{},
	}
}

func (s *fakeStore) begin() {
	s.snapUsers = map[string]domain.User{}
	for k, v := range s.users {
		s.snapUsers[k] = *v
	}
	s.snapCreds = map[string]domain.Credential{}
	for k, v := range s.credentials {
		s.snapCreds[k] = *v
	}
	s.snapSess = map[string]domain.Session{}
	for k, v := range s.sessions {
		s.snapSess[k] = *v
	}
	s.snapAuditsN = len(s.audits)
}

func (s *fakeStore) commit() {}

func (s *fakeStore) rollback() {
	s.users = map[string]*domain.User{}
	for k, v := range s.snapUsers {
		u := v
		s.users[k] = &u
	}
	s.credentials = map[string]*domain.Credential{}
	for k, v := range s.snapCreds {
		c := v
		s.credentials[k] = &c
	}
	s.sessions = map[string]*domain.Session{}
	for k, v := range s.snapSess {
		sess := v
		s.sessions[k] = &sess
	}
	s.audits = s.audits[:s.snapAuditsN]
}

type fakeUserRepo struct{ store *fakeStore }

func (r *fakeUserRepo) Create(ctx context.Context, u *domain.User) error {
	cp := *u
	r.store.users[u.ID] = &cp
	return nil
}
func (r *fakeUserRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	if u, ok := r.store.users[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, apperr.NotFound("User")
}
func (r *fakeUserRepo) GetByEmail(ctx context.Context, tenantID, email string) (*domain.User, error) {
	for _, u := range r.store.users {
		if u.Email == email && u.TenantID == tenantID {
			cp := *u
			return &cp, nil
		}
	}
	return nil, apperr.NotFound("User")
}
func (r *fakeUserRepo) FindByEmailAny(ctx context.Context, email string) ([]*domain.User, error) {
	var out []*domain.User
	for _, u := range r.store.users {
		if u.Email == email {
			cp := *u
			out = append(out, &cp)
		}
	}
	return out, nil
}
func (r *fakeUserRepo) Update(ctx context.Context, u *domain.User) error {
	cp := *u
	r.store.users[u.ID] = &cp
	return nil
}
func (r *fakeUserRepo) SoftDelete(ctx context.Context, id, by string) error { return nil }
func (r *fakeUserRepo) List(ctx context.Context, tenantID string, page ports.ListPage) ([]*domain.User, int, error) {
	return nil, 0, nil
}
func (r *fakeUserRepo) Search(ctx context.Context, tenantID string, q domain.SearchQuery) ([]*domain.User, int, error) {
	return nil, 0, nil
}

type fakeCredRepo struct{ store *fakeStore }

func (r *fakeCredRepo) Create(ctx context.Context, c *domain.Credential) error {
	cp := *c
	r.store.credentials[c.UserID] = &cp
	return nil
}
func (r *fakeCredRepo) GetByUserID(ctx context.Context, userID string) (*domain.Credential, error) {
	if c, ok := r.store.credentials[userID]; ok {
		cp := *c
		return &cp, nil
	}
	return nil, apperr.NotFound("Credential")
}
func (r *fakeCredRepo) Update(ctx context.Context, c *domain.Credential) error {
	cp := *c
	r.store.credentials[c.UserID] = &cp
	return nil
}

type fakeSessionRepo struct{ store *fakeStore }

func (r *fakeSessionRepo) Create(ctx context.Context, s *domain.Session) error {
	cp := *s
	r.store.sessions[s.ID] = &cp
	return nil
}
func (r *fakeSessionRepo) GetByTokenHash(ctx context.Context, hash string) (*domain.Session, error) {
	for _, s := range r.store.sessions {
		if s.RefreshTokenHash == hash {
			cp := *s
			return &cp, nil
		}
	}
	return nil, apperr.NotFound("Session")
}
func (r *fakeSessionRepo) GetByID(ctx context.Context, id string) (*domain.Session, error) {
	if s, ok := r.store.sessions[id]; ok {
		cp := *s
		return &cp, nil
	}
	return nil, apperr.NotFound("Session")
}
func (r *fakeSessionRepo) Update(ctx context.Context, s *domain.Session) error {
	cp := *s
	r.store.sessions[s.ID] = &cp
	return nil
}
func (r *fakeSessionRepo) ListActiveByUser(ctx context.Context, userID string) ([]*domain.Session, error) {
	var out []*domain.Session
	for _, s := range r.store.sessions {
		if s.UserID == userID && s.Status == domain.SessionActive {
			cp := *s
			out = append(out, &cp)
		}
	}
	return out, nil
}
func (r *fakeSessionRepo) RevokeFamily(ctx context.Context, familyID, reason string) error {
	now := time.Now()
	for _, s := range r.store.sessions {
		if s.FamilyID != familyID {
			continue
		}
		if s.Status == domain.SessionRotated {
			s.Status = domain.SessionReused
		} else if s.Status == domain.SessionActive {
			s.Status = domain.SessionRevoked
		} else {
			continue
		}
		s.RevokedAt = &now
		s.RevokeReason = reason
	}
	return nil
}
func (r *fakeSessionRepo) RevokeAllForUser(ctx context.Context, userID, reason string) error {
	now := time.Now()
	for _, s := range r.store.sessions {
		if s.UserID == userID && (s.Status == domain.SessionActive || s.Status == domain.SessionRotated) {
			s.Status = domain.SessionRevoked
			s.RevokedAt = &now
			s.RevokeReason = reason
		}
	}
	return nil
}

type fakeAuditRepo struct{ store *fakeStore }

func (r *fakeAuditRepo) Append(ctx context.Context, e *domain.AuditLog) error {
	cp := *e
	r.store.audits = append(r.store.audits, &cp)
	return nil
}
func (r *fakeAuditRepo) GetByID(ctx context.Context, id string) (*domain.AuditLog, error) {
	return nil, apperr.NotFound("Audit log")
}
func (r *fakeAuditRepo) List(ctx context.Context, tenantID string, page ports.ListPage) ([]*domain.AuditLog, int, error) {
	return nil, 0, nil
}

type fakeOutbox struct{}

func (fakeOutbox) Append(ctx context.Context, ev *domain.OutboxEvent) error { return nil }
func (fakeOutbox) FetchBatch(ctx context.Context, limit int, now time.Time) ([]*domain.OutboxEvent, error) {
	return nil, nil
}
func (fakeOutbox) MarkPublished(ctx context.Context, id int64, at time.Time) error { return nil }
func (fakeOutbox) MarkFailed(ctx context.Context, id int64, attempt int, next time.Time, lastErr string) error {
	return nil
}
func (fakeOutbox) MarkDead(ctx context.Context, id int64, lastErr string) error { return nil }

// ─── Fixture ────────────────────────────────────────────────────────────────

type authFixture struct {
	svc    *AuthService
	store  *fakeStore
	tx     *fakeTx
	user   *domain.User
	hasher *auth.PasswordHasher
}

const testPassword = "Corr3ct@Horse!"

func newAuthFixture(t *testing.T) *authFixture {
	t.Helper()
	store := newFakeStore()
	tx := &fakeTx{store: store}
	// Cheap Argon2 parameters: these tests hash on every login attempt.
	hasher := auth.NewPasswordHasher(auth.Argon2Params{
		Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32,
	})
	hash, err := hasher.Hash(testPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := &domain.User{
		ID: "user-1", Type: domain.UserTypeTenant, TenantID: "tenant-1",
		Email: "user@example.com", Status: domain.UserActive, Version: 1,
	}
	store.users[user.ID] = user
	store.credentials[user.ID] = &domain.Credential{UserID: user.ID, TenantID: "tenant-1", PasswordHash: hash}

	deps := &Deps{
		Tx:          tx,
		Users:       &fakeUserRepo{store: store},
		Credentials: &fakeCredRepo{store: store},
		Sessions:    &fakeSessionRepo{store: store},
		Audit:       &fakeAuditRepo{store: store},
		Outbox:      fakeOutbox{},
		Hasher:      hasher,
		JWT: auth.NewJWTManager(auth.JWTConfig{
			SigningKey: []byte("test-signing-key-at-least-32-characters-long"),
			Issuer:     "wallet-gateway", Audience: "wallet-gateway-api",
			AccessTokenTTL: time.Minute, ClientTokenTTL: time.Minute,
		}),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:    func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) },
	}
	return &authFixture{svc: NewAuthService(deps, NewAuthorizer(deps)), store: store, tx: tx, user: user, hasher: hasher}
}

// ─── Brute-force protection ─────────────────────────────────────────────────

// A wrong password must persist the attempt counter. If the rejection rolled
// the transaction back, the counter would stay at zero forever and the account
// could never lock.
func TestLoginWithWrongPasswordPersistsFailedAttempt(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	_, err := f.svc.Login(ctx, LoginCommand{Email: "user@example.com", Password: "wrong"})
	if !apperr.Is(err, apperr.CodeInvalidCredentials) {
		t.Fatalf("Login error = %v, want INVALID_CREDENTIALS", err)
	}
	cred := f.store.credentials["user-1"]
	if cred.FailedLoginAttempts != 1 {
		t.Errorf("failed_login_attempts = %d after one bad password, want 1 (the counter was rolled back)", cred.FailedLoginAttempts)
	}
	if len(f.store.audits) != 1 || f.store.audits[0].Result != domain.AuditFailure {
		t.Errorf("audit records = %d, want 1 failure record to survive the rejection", len(f.store.audits))
	}
}

func TestLoginLocksAccountAfterMaxFailedAttempts(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	for i := 0; i < domain.MaxFailedLogins; i++ {
		if _, err := f.svc.Login(ctx, LoginCommand{Email: "user@example.com", Password: "wrong"}); err == nil {
			t.Fatalf("attempt %d: Login with a wrong password succeeded", i+1)
		}
	}
	cred := f.store.credentials["user-1"]
	if cred.FailedLoginAttempts != domain.MaxFailedLogins {
		t.Errorf("failed_login_attempts = %d, want %d", cred.FailedLoginAttempts, domain.MaxFailedLogins)
	}
	if cred.LockedUntil == nil {
		t.Error("locked_until is nil, want the account to be locked")
	}
	if got := f.store.users["user-1"].Status; got != domain.UserLocked {
		t.Errorf("user status = %q, want %q", got, domain.UserLocked)
	}

	// The correct password must now be refused.
	_, err := f.svc.Login(ctx, LoginCommand{Email: "user@example.com", Password: testPassword})
	if !apperr.Is(err, apperr.CodeAccountLocked) {
		t.Fatalf("Login with the correct password on a locked account = %v, want ACCOUNT_LOCKED", err)
	}
}

func TestLoginSuccessResetsFailureCounter(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	if _, err := f.svc.Login(ctx, LoginCommand{Email: "user@example.com", Password: "wrong"}); err == nil {
		t.Fatal("expected the bad password to be rejected")
	}
	pair, err := f.svc.Login(ctx, LoginCommand{Email: "user@example.com", Password: testPassword})
	if err != nil {
		t.Fatalf("Login with the correct password: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Error("Login returned an empty token pair")
	}
	if got := f.store.credentials["user-1"].FailedLoginAttempts; got != 0 {
		t.Errorf("failed_login_attempts = %d after a successful login, want 0", got)
	}
}

// ─── Refresh rotation and reuse detection ───────────────────────────────────

func TestRefreshRotatesTheToken(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	pair, err := f.svc.Login(ctx, LoginCommand{Email: "user@example.com", Password: testPassword})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	rotated, err := f.svc.Refresh(ctx, pair.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if rotated.RefreshToken == pair.RefreshToken {
		t.Error("Refresh returned the same refresh token; it must rotate")
	}
}

// Replaying a rotated refresh token must revoke the entire family — and that
// revocation must be committed, otherwise a stolen token chain stays usable.
func TestRefreshReuseDetectionRevokesTheWholeFamily(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	pair, err := f.svc.Login(ctx, LoginCommand{Email: "user@example.com", Password: testPassword})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	rotated, err := f.svc.Refresh(ctx, pair.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// The attacker replays the old token.
	_, err = f.svc.Refresh(ctx, pair.RefreshToken)
	if !apperr.Is(err, apperr.CodeTokenReuse) {
		t.Fatalf("replaying a rotated token = %v, want TOKEN_REUSE_DETECTED", err)
	}

	// The legitimate token from the same family must now be dead too.
	if _, err := f.svc.Refresh(ctx, rotated.RefreshToken); err == nil {
		t.Fatal("the rotated token still works after reuse detection: the family revocation was rolled back")
	}
	for id, s := range f.store.sessions {
		if s.Status == domain.SessionActive {
			t.Errorf("session %s is still active after family revocation", id)
		}
	}
}

func TestRefreshRejectsAnUnknownToken(t *testing.T) {
	f := newAuthFixture(t)
	if _, err := f.svc.Refresh(context.Background(), "not-a-real-token"); err == nil {
		t.Fatal("Refresh accepted an unknown token")
	}
}

// ─── Password change ────────────────────────────────────────────────────────

func TestChangePasswordWithWrongCurrentPasswordIsAudited(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	principal := domain.Principal{ID: "user-1", Kind: domain.KindTenantUser, TenantID: "tenant-1"}

	err := f.svc.ChangePassword(ctx, principal, ChangePasswordCommand{
		CurrentPassword: "wrong", NewPassword: "An0ther@Str0ng!",
	})
	if !apperr.Is(err, apperr.CodeInvalidCredentials) {
		t.Fatalf("ChangePassword = %v, want INVALID_CREDENTIALS", err)
	}
	if len(f.store.audits) != 1 {
		t.Fatalf("audit records = %d, want the failed attempt to be recorded", len(f.store.audits))
	}
	if f.store.audits[0].Result != domain.AuditFailure {
		t.Errorf("audit result = %q, want failure", f.store.audits[0].Result)
	}
}

func TestChangePasswordEnforcesThePolicy(t *testing.T) {
	f := newAuthFixture(t)
	principal := domain.Principal{ID: "user-1", Kind: domain.KindTenantUser, TenantID: "tenant-1"}

	err := f.svc.ChangePassword(context.Background(), principal, ChangePasswordCommand{
		CurrentPassword: testPassword, NewPassword: "weak",
	})
	if !apperr.Is(err, apperr.CodeValidation) {
		t.Fatalf("ChangePassword with a weak password = %v, want VALIDATION_ERROR", err)
	}
}

func TestChangePasswordRotatesTheHashAndClearsMustChange(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	f.store.users["user-1"].MustChangePassword = true
	oldHash := f.store.credentials["user-1"].PasswordHash
	principal := domain.Principal{ID: "user-1", Kind: domain.KindTenantUser, TenantID: "tenant-1"}

	if err := f.svc.ChangePassword(ctx, principal, ChangePasswordCommand{
		CurrentPassword: testPassword, NewPassword: "An0ther@Str0ng!",
	}); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if f.store.credentials["user-1"].PasswordHash == oldHash {
		t.Error("the password hash was not rotated")
	}
	if f.store.users["user-1"].MustChangePassword {
		t.Error("must_change_password is still true after a successful change")
	}
	// The new password must work, the old one must not.
	if _, err := f.svc.Login(ctx, LoginCommand{Email: "user@example.com", Password: "An0ther@Str0ng!"}); err != nil {
		t.Errorf("login with the new password failed: %v", err)
	}
	if _, err := f.svc.Login(ctx, LoginCommand{Email: "user@example.com", Password: testPassword}); err == nil {
		t.Error("the old password still works after a password change")
	}
}

// ─── Account status ─────────────────────────────────────────────────────────

func TestLoginRejectsNonActiveAccounts(t *testing.T) {
	tests := []struct {
		status   domain.UserStatus
		wantCode apperr.Code
	}{
		{domain.UserSuspended, apperr.CodePermissionDenied},
		{domain.UserDisabled, apperr.CodePermissionDenied},
		{domain.UserLocked, apperr.CodeAccountLocked},
	}
	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			f := newAuthFixture(t)
			f.store.users["user-1"].Status = tc.status
			_, err := f.svc.Login(context.Background(), LoginCommand{Email: "user@example.com", Password: testPassword})
			if !apperr.Is(err, tc.wantCode) {
				t.Fatalf("Login for a %s account = %v, want %s", tc.status, err, tc.wantCode)
			}
		})
	}
}

func TestLoginWithUnknownEmailIsIndistinguishable(t *testing.T) {
	f := newAuthFixture(t)
	_, err := f.svc.Login(context.Background(), LoginCommand{Email: "nobody@example.com", Password: testPassword})
	if !apperr.Is(err, apperr.CodeInvalidCredentials) {
		t.Fatalf("Login with an unknown email = %v, want INVALID_CREDENTIALS (no user enumeration)", err)
	}
	var appErr *apperr.AppError
	if errors.As(err, &appErr) && appErr.Detail != "Email or password is incorrect." {
		t.Errorf("detail = %q, want the generic credentials message", appErr.Detail)
	}
}
