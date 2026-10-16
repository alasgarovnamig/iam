//go:build integration

// Package integration exercises the API against the real stack
// (PostgreSQL + Redis + OPA + OpenFGA + Kafka) started by docker compose.
//
//	make docker-up && make test-integration
//
// These tests deliberately go through HTTP: the guarantees they check —
// tenant isolation, field-level filtering, delegation, idempotency,
// optimistic locking — only hold when every layer is wired together.
package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

var (
	apiURL        = envOr("API_URL", "http://localhost:8090")
	adminEmail    = envOr("BOOTSTRAP_ADMIN_EMAIL", "sr.alasgarov@gmail.com")
	adminPassword = envOr("E2E_ADMIN_PASSWORD", "E2e@Str0ngPass1")
	// Used only when the deployment has never had its password changed.
	bootstrapPassword = envOr("BOOTSTRAP_ADMIN_PASSWORD", "Anh0@111118")
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ─── HTTP helpers ───────────────────────────────────────────────────────────

type client struct {
	t     *testing.T
	token string
}

type apiResponse struct {
	status int
	body   map[string]any
	raw    []byte
}

func (c *client) do(method, path string, body any, headers map[string]string) apiResponse {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, apiURL+path, reader)
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v (is the stack up? run: make docker-up)", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := apiResponse{status: resp.StatusCode, raw: raw}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out.body)
	}
	return out
}

// data returns the "data" object of a success envelope.
func (r apiResponse) data(t *testing.T) map[string]any {
	t.Helper()
	d, ok := r.body["data"].(map[string]any)
	if !ok {
		t.Fatalf("response has no data object: %s", string(r.raw))
	}
	return d
}

func (r apiResponse) code() string {
	if c, ok := r.body["code"].(string); ok {
		return c
	}
	return ""
}

// login authenticates and returns a client carrying the access token.
//
// Sign-ins are deliberately scarce here: login is rate limited per IP (10/min)
// as brute-force protection, so tests share tokens through the `shared*`
// helpers below instead of authenticating one-per-test.
func login(t *testing.T, email, password, tenantSlug string) *client {
	t.Helper()
	token, err := signIn(email, password, tenantSlug)
	if err != nil {
		t.Fatalf("login %s: %v", email, err)
	}
	return &client{t: t, token: token}
}

// signIn performs a login without a *testing.T, so package-level fixtures can
// use it from sync.Once.
func signIn(email, password, tenantSlug string) (string, error) {
	body := map[string]any{"email": email, "password": password}
	if tenantSlug != "" {
		body["tenant_slug"] = tenantSlug
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Post(
		apiURL+"/api/v1/auth/login", "application/json", bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("%w (is the stack up? run: make docker-up)", err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(payload))
	}
	var out struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		return "", err
	}
	if out.Data.AccessToken == "" {
		return "", fmt.Errorf("no access token in response")
	}
	return out.Data.AccessToken, nil
}

var (
	adminOnce  sync.Once
	adminToken string
	adminErr   error
)

// adminClient returns the shared platform-admin session, signing in once per
// test binary and performing the one-time forced password change when this
// deployment is brand new.
func adminClient(t *testing.T) *client {
	t.Helper()
	adminOnce.Do(func() { adminToken, adminErr = signInAdmin() })
	if adminErr != nil {
		t.Fatalf("cannot sign in as the platform admin: %v", adminErr)
	}
	return &client{t: t, token: adminToken}
}

func signInAdmin() (string, error) {
	if token, err := signIn(adminEmail, adminPassword, ""); err == nil {
		return token, nil
	}
	// Fresh deployment: the bootstrap password is still in place and must be
	// changed before anything else works.
	token, err := signIn(adminEmail, bootstrapPassword, "")
	if err != nil {
		return "", err
	}
	boot := &client{token: token}
	raw, err := json.Marshal(map[string]any{
		"current_password": bootstrapPassword, "new_password": adminPassword,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, apiURL+"/api/v1/auth/change-password", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+boot.token)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("forced password change failed: %d %s", resp.StatusCode, string(body))
	}
	return signIn(adminEmail, adminPassword, "")
}

func unique() string { return uuid.NewString()[:8] }

// ─── Fixtures ───────────────────────────────────────────────────────────────

type tenantFixture struct {
	id        string
	slug      string
	adminID   string
	adminMail string
	adminPass string
}

// newTenant creates a tenant plus a Tenant Admin user inside it.
func newTenant(t *testing.T, admin *client) tenantFixture {
	t.Helper()
	slug := "it-" + unique()
	resp := admin.do(http.MethodPost, "/api/v1/tenants",
		map[string]any{"name": "Integration " + slug, "slug": slug}, nil)
	if resp.status != http.StatusCreated {
		t.Fatalf("create tenant: %d %s", resp.status, string(resp.raw))
	}
	tenantID, _ := resp.data(t)["id"].(string)

	email := fmt.Sprintf("admin-%s@example.test", unique())
	const password = "Tenant@Str0ng!1"
	userResp := admin.do(http.MethodPost, "/api/v1/users", map[string]any{
		"tenant_id": tenantID, "email": email, "password": password,
		"first_name": "Tenant", "last_name": "Admin",
		// Tenant Admin system role.
		"role_ids": []string{"a2000000-0000-4000-8000-000000000002"},
	}, nil)
	if userResp.status != http.StatusCreated {
		t.Fatalf("create tenant admin: %d %s", userResp.status, string(userResp.raw))
	}
	userID, _ := userResp.data(t)["id"].(string)

	return tenantFixture{id: tenantID, slug: slug, adminID: userID, adminMail: email, adminPass: password}
}

var (
	primaryOnce   sync.Once
	primaryTenant tenantFixture
	primaryClient *client

	secondaryOnce   sync.Once
	secondaryTenant tenantFixture
)

// sharedTenant returns a tenant plus a signed-in Tenant Admin, created once
// per test binary. Tests that only need "a tenant" reuse this one so the suite
// stays under the login rate limit.
func sharedTenant(t *testing.T) (tenantFixture, *client) {
	t.Helper()
	admin := adminClient(t)
	primaryOnce.Do(func() {
		primaryTenant = newTenant(t, admin)
		token, err := signIn(primaryTenant.adminMail, primaryTenant.adminPass, primaryTenant.slug)
		if err != nil {
			t.Fatalf("sign in as the tenant admin: %v", err)
		}
		primaryClient = &client{token: token}
	})
	if primaryClient == nil {
		t.Fatal("the shared tenant fixture failed to initialise")
	}
	return primaryTenant, &client{t: t, token: primaryClient.token}
}

// foreignTenant returns a second tenant, for isolation tests. No session is
// needed for it: the point is that the primary tenant cannot reach its data.
func foreignTenant(t *testing.T) tenantFixture {
	t.Helper()
	admin := adminClient(t)
	secondaryOnce.Do(func() { secondaryTenant = newTenant(t, admin) })
	return secondaryTenant
}

// permissionID looks up a catalog entry by resource/action/field.
func permissionID(t *testing.T, c *client, resource, action, field string) string {
	t.Helper()
	resp := c.do(http.MethodGet, "/api/v1/permissions", nil, nil)
	if resp.status != http.StatusOK {
		t.Fatalf("read catalog: %d %s", resp.status, string(resp.raw))
	}
	list, _ := resp.body["data"].([]any)
	for _, item := range list {
		def, _ := item.(map[string]any)
		if def["resource"] == resource && def["action"] == action {
			defField, _ := def["field"].(string)
			if defField == field {
				id, _ := def["id"].(string)
				return id
			}
		}
	}
	t.Fatalf("permission %s.%s.%s not found in the catalog", resource, action, field)
	return ""
}

// ─── Tenant isolation ───────────────────────────────────────────────────────

func TestSuperAdminCreatesTenant(t *testing.T) {
	admin := adminClient(t)
	slug := "it-" + unique()

	resp := admin.do(http.MethodPost, "/api/v1/tenants",
		map[string]any{"name": "Acme " + slug, "slug": slug}, nil)

	if resp.status != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", resp.status, string(resp.raw))
	}
	if got := resp.data(t)["slug"]; got != slug {
		t.Errorf("slug = %v, want %q", got, slug)
	}
	if got := resp.data(t)["status"]; got != "active" {
		t.Errorf("status = %v, want active", got)
	}
}

func TestTenantAdminCreatesUserInOwnTenant(t *testing.T) {
	tenant, tenantAdmin := sharedTenant(t)

	resp := tenantAdmin.do(http.MethodPost, "/api/v1/users", map[string]any{
		"email":    fmt.Sprintf("staff-%s@example.test", unique()),
		"password": "Staff@Str0ng!1", "first_name": "Staff",
	}, nil)

	if resp.status != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", resp.status, string(resp.raw))
	}
	if got := resp.data(t)["tenant_id"]; got != tenant.id {
		t.Errorf("tenant_id = %v, want the actor's own tenant %q", got, tenant.id)
	}
}

func TestTenantAdminCannotCreateUserInAnotherTenant(t *testing.T) {
	_, actor := sharedTenant(t)
	tenantB := foreignTenant(t)

	resp := actor.do(http.MethodPost, "/api/v1/users", map[string]any{
		"tenant_id": tenantB.id,
		"email":     fmt.Sprintf("evil-%s@example.test", unique()),
		"password":  "Evil@Str0ng!1",
	}, nil)

	if resp.status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", resp.status, string(resp.raw))
	}
}

func TestTenantUserCannotReadForeignRecordEvenKnowingItsID(t *testing.T) {
	_, actor := sharedTenant(t)
	tenantB := foreignTenant(t)

	// tenantB.adminID is a real id; row-level security must still hide it.
	resp := actor.do(http.MethodGet, "/api/v1/users/"+tenantB.adminID, nil, nil)

	if resp.status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (no cross-tenant read, no existence leak): %s",
			resp.status, string(resp.raw))
	}
}

func TestTenantListIsScopedToOwnTenant(t *testing.T) {
	tenantA, actor := sharedTenant(t)
	_ = foreignTenant(t) // a second tenant must not appear in the list

	resp := actor.do(http.MethodGet, "/api/v1/users?page_size=100", nil, nil)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.status, string(resp.raw))
	}
	users, _ := resp.body["data"].([]any)
	for _, item := range users {
		u, _ := item.(map[string]any)
		if u["tenant_id"] != tenantA.id {
			t.Errorf("list returned a user from tenant %v; only %q is allowed", u["tenant_id"], tenantA.id)
		}
	}
}

// ─── Delegation ─────────────────────────────────────────────────────────────

func TestDelegationAllowsGrantingAHeldPermission(t *testing.T) {
	_, actor := sharedTenant(t)

	role := actor.do(http.MethodPost, "/api/v1/roles",
		map[string]any{"name": "Delegation OK " + unique()}, nil)
	if role.status != http.StatusCreated {
		t.Fatalf("create role: %d %s", role.status, string(role.raw))
	}
	roleID, _ := role.data(t)["id"].(string)

	// The Tenant Admin role holds users.create at tenant scope.
	resp := actor.do(http.MethodPost, "/api/v1/roles/"+roleID+"/permissions", map[string]any{
		"permission_id": permissionID(t, actor, "users", "create", ""),
		"scope":         "tenant",
		"effect":        "allow",
	}, nil)

	if resp.status != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", resp.status, string(resp.raw))
	}
}

func TestDelegationRejectsPlatformOnlyPermissionOnTenantRole(t *testing.T) {
	// The platform-only definition is invisible to the tenant catalog, so its
	// id is resolved with the admin session.
	admin := adminClient(t)
	_, actor := sharedTenant(t)

	role := actor.do(http.MethodPost, "/api/v1/roles",
		map[string]any{"name": "Escalation " + unique()}, nil)
	roleID, _ := role.data(t)["id"].(string)

	// tenants.create is platform-only: a tenant admin does not hold it.
	resp := actor.do(http.MethodPost, "/api/v1/roles/"+roleID+"/permissions", map[string]any{
		"permission_id": permissionID(t, admin, "tenants", "create", ""),
		"scope":         "tenant",
		"effect":        "allow",
	}, nil)

	if resp.status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", resp.status, string(resp.raw))
	}
	if resp.code() != "PERMISSION_DELEGATION_DENIED" {
		t.Errorf("code = %q, want PERMISSION_DELEGATION_DENIED", resp.code())
	}
}

func TestDelegationRejectsWiderScopeThanHeld(t *testing.T) {
	_, actor := sharedTenant(t)

	role := actor.do(http.MethodPost, "/api/v1/roles",
		map[string]any{"name": "Wider scope " + unique()}, nil)
	roleID, _ := role.data(t)["id"].(string)

	// The tenant admin holds users.read at scope=tenant, never scope=all.
	resp := actor.do(http.MethodPost, "/api/v1/roles/"+roleID+"/permissions", map[string]any{
		"permission_id": permissionID(t, actor, "users", "read", ""),
		"scope":         "all",
		"effect":        "allow",
	}, nil)

	if resp.status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", resp.status, string(resp.raw))
	}
	if resp.code() != "PERMISSION_DELEGATION_DENIED" {
		t.Errorf("code = %q, want PERMISSION_DELEGATION_DENIED", resp.code())
	}
}

// A privilege-escalation attempt must leave a trace even though the business
// transaction rolls back.
func TestDeniedDelegationIsAudited(t *testing.T) {
	admin := adminClient(t)
	_, actor := sharedTenant(t)

	role := actor.do(http.MethodPost, "/api/v1/roles",
		map[string]any{"name": "Audited escalation " + unique()}, nil)
	roleID, _ := role.data(t)["id"].(string)

	denied := actor.do(http.MethodPost, "/api/v1/roles/"+roleID+"/permissions", map[string]any{
		"permission_id": permissionID(t, admin, "tenants", "create", ""),
		"scope":         "tenant",
	}, nil)
	if denied.status != http.StatusForbidden {
		t.Fatalf("setup: expected the delegation to be denied, got %d", denied.status)
	}

	audit := actor.do(http.MethodGet,
		"/api/v1/audit-logs?filter%5Bresult%5D=denied&page_size=50", nil, nil)
	if audit.status != http.StatusOK {
		t.Fatalf("read audit logs: %d %s", audit.status, string(audit.raw))
	}
	entries, _ := audit.body["data"].([]any)
	for _, item := range entries {
		e, _ := item.(map[string]any)
		if e["resource_id"] == roleID {
			return // the attempt was recorded
		}
	}
	t.Error("the rejected delegation attempt was not recorded in the audit log")
}

// ─── Field-level permissions ────────────────────────────────────────────────

// buildLimitedUser creates a user whose custom role grants users.read/list plus
// only the named read/search fields.
func buildLimitedUser(t *testing.T, actor *client, readFields, searchFields []string) *client {
	t.Helper()
	role := actor.do(http.MethodPost, "/api/v1/roles",
		map[string]any{"name": "Limited " + unique()}, nil)
	if role.status != http.StatusCreated {
		t.Fatalf("create role: %d %s", role.status, string(role.raw))
	}
	roleID, _ := role.data(t)["id"].(string)

	grant := func(resource, action, field string) {
		resp := actor.do(http.MethodPost, "/api/v1/roles/"+roleID+"/permissions", map[string]any{
			"permission_id": permissionID(t, actor, resource, action, field),
			"scope":         "tenant",
			"effect":        "allow",
		}, nil)
		if resp.status != http.StatusCreated {
			t.Fatalf("grant %s.%s.%s: %d %s", resource, action, field, resp.status, string(resp.raw))
		}
	}
	grant("users", "read", "")
	grant("users", "list", "")
	for _, f := range readFields {
		grant("users", "read", f)
	}
	if len(searchFields) > 0 {
		grant("users", "search", "")
		for _, f := range searchFields {
			grant("users", "search", f)
		}
	}

	email := fmt.Sprintf("limited-%s@example.test", unique())
	const password = "Limited@Str0ng!1"
	user := actor.do(http.MethodPost, "/api/v1/users", map[string]any{
		"email": email, "password": password, "first_name": "Limited",
		"role_ids": []string{roleID},
	}, nil)
	if user.status != http.StatusCreated {
		t.Fatalf("create limited user: %d %s", user.status, string(user.raw))
	}

	return loginWithTenantOf(t, actor, email, password)
}

// loginWithTenantOf signs in a user of the same tenant as `actor`.
func loginWithTenantOf(t *testing.T, actor *client, email, password string) *client {
	t.Helper()
	tenants := actor.do(http.MethodGet, "/api/v1/tenants?page_size=1", nil, nil)
	slug := ""
	if items, ok := tenants.body["data"].([]any); ok && len(items) > 0 {
		if tenant, ok := items[0].(map[string]any); ok {
			slug, _ = tenant["slug"].(string)
		}
	}
	return login(t, email, password, slug)
}

func TestReadFieldPermissionHidesUngrantedFields(t *testing.T) {
	tenant, actor := sharedTenant(t)

	// Can read email and first_name — but never phone.
	limited := buildLimitedUser(t, actor, []string{"email", "first_name"}, nil)

	resp := limited.do(http.MethodGet, "/api/v1/users/"+tenant.adminID, nil, nil)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.status, string(resp.raw))
	}
	user := resp.data(t)
	if _, ok := user["email"]; !ok {
		t.Error("email is absent although users.read.email was granted")
	}
	if _, ok := user["phone"]; ok {
		t.Error("phone is present although users.read.phone was NOT granted")
	}
	if _, ok := user["password_hash"]; ok {
		t.Error("password_hash must never appear in an API response")
	}
	if _, ok := user["internal_notes"]; ok {
		t.Error("internal_notes is present although it was not granted")
	}
}

func TestSearchFieldPermissionIsRequiredToFilter(t *testing.T) {
	tenant, actor := sharedTenant(t)

	// Reads email, but may only search by first_name.
	limited := buildLimitedUser(t, actor, []string{"email", "first_name"}, []string{"first_name"})

	allowed := limited.do(http.MethodGet, "/api/v1/users?filter%5Bfirst_name%5D=Tenant", nil, nil)
	if allowed.status != http.StatusOK {
		t.Errorf("filtering by first_name = %d, want 200 (search.first_name granted): %s",
			allowed.status, string(allowed.raw))
	}

	// Reading email does not imply searching by it.
	denied := limited.do(http.MethodGet, "/api/v1/users?filter%5Bemail%5D="+tenant.adminMail, nil, nil)
	if denied.status != http.StatusForbidden {
		t.Fatalf("filtering by email = %d, want 403: %s", denied.status, string(denied.raw))
	}
	if denied.code() != "FIELD_NOT_ALLOWED" {
		t.Errorf("code = %q, want FIELD_NOT_ALLOWED", denied.code())
	}
}

func TestUpdateRejectsFieldWithoutFieldPermission(t *testing.T) {
	tenant, actor := sharedTenant(t)

	// The Tenant Admin role deliberately lacks users.update.internal_notes.
	resp := actor.do(http.MethodPatch, "/api/v1/users/"+tenant.adminID,
		map[string]any{"internal_notes": "should be rejected"}, nil)

	if resp.status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", resp.status, string(resp.raw))
	}
	if resp.code() != "FIELD_NOT_ALLOWED" {
		t.Errorf("code = %q, want FIELD_NOT_ALLOWED", resp.code())
	}
}

func TestUnknownBodyFieldIsRejected(t *testing.T) {
	_, actor := sharedTenant(t)

	resp := actor.do(http.MethodPost, "/api/v1/users", map[string]any{
		"email":    fmt.Sprintf("mass-%s@example.test", unique()),
		"password": "Mass@Str0ng!1",
		// Not part of the contract: mass-assignment protection must reject it.
		"is_super_admin": true,
	}, nil)

	if resp.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.status, string(resp.raw))
	}
}

// ─── Idempotency and optimistic locking ─────────────────────────────────────

func TestDuplicateIdempotencyKeyDoesNotCreateTwoUsers(t *testing.T) {
	_, actor := sharedTenant(t)

	key := uuid.NewString()
	body := map[string]any{
		"email":    fmt.Sprintf("idem-%s@example.test", unique()),
		"password": "Idem@Str0ng!1", "first_name": "Idem",
	}
	headers := map[string]string{"Idempotency-Key": key}

	first := actor.do(http.MethodPost, "/api/v1/users", body, headers)
	if first.status != http.StatusCreated {
		t.Fatalf("first create: %d %s", first.status, string(first.raw))
	}
	firstID, _ := first.data(t)["id"].(string)

	second := actor.do(http.MethodPost, "/api/v1/users", body, headers)
	if second.status != http.StatusCreated {
		t.Fatalf("replay: status = %d, want the stored 201: %s", second.status, string(second.raw))
	}
	secondID, _ := second.data(t)["id"].(string)

	if firstID != secondID {
		t.Errorf("replay created a second user (%s vs %s); the key must replay the first response",
			firstID, secondID)
	}
}

func TestSameIdempotencyKeyWithDifferentBodyConflicts(t *testing.T) {
	_, actor := sharedTenant(t)

	key := uuid.NewString()
	headers := map[string]string{"Idempotency-Key": key}

	first := actor.do(http.MethodPost, "/api/v1/users", map[string]any{
		"email": fmt.Sprintf("idem-a-%s@example.test", unique()), "password": "Idem@Str0ng!1",
	}, headers)
	if first.status != http.StatusCreated {
		t.Fatalf("first create: %d %s", first.status, string(first.raw))
	}

	second := actor.do(http.MethodPost, "/api/v1/users", map[string]any{
		"email": fmt.Sprintf("idem-b-%s@example.test", unique()), "password": "Idem@Str0ng!1",
	}, headers)

	if second.status != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", second.status, string(second.raw))
	}
	if second.code() != "IDEMPOTENCY_CONFLICT" {
		t.Errorf("code = %q, want IDEMPOTENCY_CONFLICT", second.code())
	}
}

func TestStaleVersionCausesOptimisticLockConflict(t *testing.T) {
	_, actor := sharedTenant(t)

	role := actor.do(http.MethodPost, "/api/v1/roles",
		map[string]any{"name": "Concurrent " + unique()}, nil)
	roleID, _ := role.data(t)["id"].(string)
	version, _ := role.data(t)["version"].(float64)

	first := actor.do(http.MethodPatch, "/api/v1/roles/"+roleID,
		map[string]any{"description": "first writer", "version": version}, nil)
	if first.status != http.StatusOK {
		t.Fatalf("first update: %d %s", first.status, string(first.raw))
	}

	// The second writer still holds the old version.
	second := actor.do(http.MethodPatch, "/api/v1/roles/"+roleID,
		map[string]any{"description": "second writer", "version": version}, nil)

	if second.status != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", second.status, string(second.raw))
	}
	if second.code() != "VERSION_CONFLICT" {
		t.Errorf("code = %q, want VERSION_CONFLICT", second.code())
	}
}

// ─── System roles ───────────────────────────────────────────────────────────

func TestSystemRolesAreImmutable(t *testing.T) {
	admin := adminClient(t)
	const superAdminRoleID = "a1000000-0000-4000-8000-000000000001"

	resp := admin.do(http.MethodPatch, "/api/v1/roles/"+superAdminRoleID,
		map[string]any{"description": "tampered"}, nil)

	if resp.status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 even for a super admin: %s", resp.status, string(resp.raw))
	}
}

// ─── Audit hygiene ──────────────────────────────────────────────────────────

func TestAuditLogNeverStoresCredentials(t *testing.T) {
	admin := adminClient(t)
	tenant, _ := sharedTenant(t)

	resp := admin.do(http.MethodGet, "/api/v1/audit-logs?page_size=100", nil, nil)
	if resp.status != http.StatusOK {
		t.Fatalf("read audit logs: %d %s", resp.status, string(resp.raw))
	}
	body := string(resp.raw)
	for _, secret := range []string{tenant.adminPass, adminPassword, bootstrapPassword, "$argon2id$"} {
		if bytes.Contains([]byte(body), []byte(secret)) {
			t.Errorf("the audit log leaked credential material (%q)", secret)
		}
	}
}
