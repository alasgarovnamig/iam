package httpapi

import (
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/sr-alasgarov/wallet-gateway/internal/app"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
)

// ResourceHandlers serves tenants, users, roles, permissions, api-clients
// and audit-log endpoints.
type ResourceHandlers struct {
	Tenants     *app.TenantService
	Users       *app.UserService
	Roles       *app.RoleService
	Permissions *app.PermissionService
	APIClients  *app.APIClientService
	AuditLogs   *app.AuditService
	Logger      *slog.Logger
}

// ─── Tenants ────────────────────────────────────────────────────────────────

type createTenantRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (h *ResourceHandlers) CreateTenant(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var req createTenantRequest
	if _, err := decodeBody(r, &req); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	tenant, err := h.Tenants.Create(r.Context(), p, app.CreateTenantCommand{Name: req.Name, Slug: req.Slug})
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondData(w, r, http.StatusCreated, toTenantDTO(tenant))
}

func (h *ResourceHandlers) ListTenants(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	page := parseListPage(r)
	tenants, total, err := h.Tenants.List(r.Context(), p, page)
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	out := make([]tenantDTO, len(tenants))
	for i, t := range tenants {
		out[i] = toTenantDTO(t)
	}
	respondList(w, r, out, page.Page, page.PageSize, total)
}

func (h *ResourceHandlers) GetTenant(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	tenant, err := h.Tenants.Get(r.Context(), p, chi.URLParam(r, "tenantId"))
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondData(w, r, http.StatusOK, toTenantDTO(tenant))
}

type updateTenantRequest struct {
	Name    *string `json:"name"`
	Status  *string `json:"status"`
	Version int64   `json:"version"`
}

func (h *ResourceHandlers) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var req updateTenantRequest
	if _, err := decodeBody(r, &req); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	tenant, err := h.Tenants.Update(r.Context(), p, chi.URLParam(r, "tenantId"),
		app.UpdateTenantCommand{Name: req.Name, Status: req.Status, Version: req.Version})
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondData(w, r, http.StatusOK, toTenantDTO(tenant))
}

func (h *ResourceHandlers) DeleteTenant(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if err := h.Tenants.Delete(r.Context(), p, chi.URLParam(r, "tenantId")); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondNoContent(w)
}

// ─── Users ──────────────────────────────────────────────────────────────────

type createUserRequest struct {
	TenantID  string   `json:"tenant_id"`
	Email     string   `json:"email"`
	Password  string   `json:"password"`
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	Phone     string   `json:"phone"`
	RoleIDs   []string `json:"role_ids"`
}

func (h *ResourceHandlers) CreateUser(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var req createUserRequest
	provided, err := decodeBody(r, &req)
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	view, err := h.Users.Create(r.Context(), p, app.CreateUserCommand{
		TenantID: req.TenantID, Email: req.Email, Password: req.Password,
		FirstName: req.FirstName, LastName: req.LastName, Phone: req.Phone,
		RoleIDs: req.RoleIDs, ProvidedFields: provided,
	})
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondData(w, r, http.StatusCreated, userDTO(view.User, view.AllowedFields, false))
}

func (h *ResourceHandlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	page := parseListPage(r)
	views, total, err := h.Users.List(r.Context(), p, app.ListUsersQuery{
		TenantID: r.URL.Query().Get("tenant_id"), Page: page,
	})
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	out := make([]map[string]any, len(views))
	for i, v := range views {
		out[i] = userDTO(v.User, v.AllowedFields, v.User.ID == p.ID)
	}
	respondList(w, r, out, page.Page, page.PageSize, total)
}

func (h *ResourceHandlers) GetUser(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	view, err := h.Users.Get(r.Context(), p, chi.URLParam(r, "userId"))
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondData(w, r, http.StatusOK, userDTO(view.User, view.AllowedFields, view.User.ID == p.ID))
}

type updateUserRequest struct {
	Email         *string `json:"email"`
	FirstName     *string `json:"first_name"`
	LastName      *string `json:"last_name"`
	Phone         *string `json:"phone"`
	InternalNotes *string `json:"internal_notes"`
	Version       int64   `json:"version"`
}

func (h *ResourceHandlers) UpdateUser(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var req updateUserRequest
	provided, err := decodeBody(r, &req)
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	view, err := h.Users.Update(r.Context(), p, chi.URLParam(r, "userId"), app.UpdateUserCommand{
		Email: req.Email, FirstName: req.FirstName, LastName: req.LastName,
		Phone: req.Phone, InternalNotes: req.InternalNotes, Version: req.Version,
		ProvidedFields: provided,
	})
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondData(w, r, http.StatusOK, userDTO(view.User, view.AllowedFields, view.User.ID == p.ID))
}

func (h *ResourceHandlers) DeleteUser(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if err := h.Users.Delete(r.Context(), p, chi.URLParam(r, "userId")); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondNoContent(w)
}

func (h *ResourceHandlers) userLifecycle(action func(*http.Request, domain.Principal, string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, _ := principalFrom(r.Context())
		if err := action(r, p, chi.URLParam(r, "userId")); err != nil {
			respondError(w, r, err, h.Logger)
			return
		}
		respondNoContent(w)
	}
}

func (h *ResourceHandlers) ActivateUser() http.HandlerFunc {
	return h.userLifecycle(func(r *http.Request, p domain.Principal, id string) error {
		return h.Users.Activate(r.Context(), p, id)
	})
}

func (h *ResourceHandlers) SuspendUser() http.HandlerFunc {
	return h.userLifecycle(func(r *http.Request, p domain.Principal, id string) error {
		return h.Users.Suspend(r.Context(), p, id)
	})
}

func (h *ResourceHandlers) UnlockUser() http.HandlerFunc {
	return h.userLifecycle(func(r *http.Request, p domain.Principal, id string) error {
		return h.Users.Unlock(r.Context(), p, id)
	})
}

func (h *ResourceHandlers) UserEffectivePermissions(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	perms, err := h.Users.EffectivePermissions(r.Context(), p, chi.URLParam(r, "userId"))
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	out := make([]permissionGrantDTO, len(perms))
	for i, perm := range perms {
		out[i] = toGrantDTO("", perm)
	}
	respondData(w, r, http.StatusOK, out)
}

// ─── Roles ──────────────────────────────────────────────────────────────────

type createRoleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	TenantID    string `json:"tenant_id"`
}

func (h *ResourceHandlers) CreateRole(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var req createRoleRequest
	if _, err := decodeBody(r, &req); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	role, err := h.Roles.Create(r.Context(), p, app.CreateRoleCommand{
		Name: req.Name, Description: req.Description, TenantID: req.TenantID,
	})
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondData(w, r, http.StatusCreated, toRoleDTO(role))
}

func (h *ResourceHandlers) ListRoles(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	page := parseListPage(r)
	roles, total, err := h.Roles.List(r.Context(), p, page)
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	out := make([]roleDTO, len(roles))
	for i, role := range roles {
		out[i] = toRoleDTO(role)
	}
	respondList(w, r, out, page.Page, page.PageSize, total)
}

func (h *ResourceHandlers) GetRole(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	role, perms, err := h.Roles.Get(r.Context(), p, chi.URLParam(r, "roleId"))
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	grants := make([]permissionGrantDTO, len(perms))
	for i, rp := range perms {
		grants[i] = toGrantDTO(rp.ID, rp.Permission)
	}
	respondData(w, r, http.StatusOK, map[string]any{
		"role":        toRoleDTO(role),
		"permissions": grants,
	})
}

type updateRoleRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
	Version     int64   `json:"version"`
}

func (h *ResourceHandlers) UpdateRole(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var req updateRoleRequest
	if _, err := decodeBody(r, &req); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	role, err := h.Roles.Update(r.Context(), p, chi.URLParam(r, "roleId"), app.UpdateRoleCommand{
		Name: req.Name, Description: req.Description, Status: req.Status, Version: req.Version,
	})
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondData(w, r, http.StatusOK, toRoleDTO(role))
}

func (h *ResourceHandlers) DeleteRole(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if err := h.Roles.Delete(r.Context(), p, chi.URLParam(r, "roleId")); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondNoContent(w)
}

type grantPermissionRequest struct {
	PermissionID string            `json:"permission_id"`
	Scope        string            `json:"scope"`
	Effect       string            `json:"effect"`
	Conditions   map[string]string `json:"conditions"`
}

func (h *ResourceHandlers) AddRolePermission(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var req grantPermissionRequest
	if _, err := decodeBody(r, &req); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	rp, err := h.Roles.AddPermission(r.Context(), p, chi.URLParam(r, "roleId"),
		app.GrantRolePermissionCommand{
			PermissionID: req.PermissionID, Scope: req.Scope, Effect: req.Effect, Conditions: req.Conditions,
		})
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondData(w, r, http.StatusCreated, toGrantDTO(rp.ID, rp.Permission))
}

func (h *ResourceHandlers) RemoveRolePermission(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if err := h.Roles.RemovePermission(r.Context(), p, chi.URLParam(r, "roleId"), chi.URLParam(r, "permissionId")); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondNoContent(w)
}

type assignRoleRequest struct {
	RoleID string `json:"role_id"`
}

func (h *ResourceHandlers) AssignRoleToUser(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var req assignRoleRequest
	if _, err := decodeBody(r, &req); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	if err := h.Roles.AssignToUser(r.Context(), p, chi.URLParam(r, "userId"), req.RoleID); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondData(w, r, http.StatusCreated, map[string]any{"user_id": chi.URLParam(r, "userId"), "role_id": req.RoleID})
}

func (h *ResourceHandlers) RevokeRoleFromUser(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if err := h.Roles.RevokeFromUser(r.Context(), p, chi.URLParam(r, "userId"), chi.URLParam(r, "roleId")); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondNoContent(w)
}

// ─── Permission catalog + direct grants ─────────────────────────────────────

func (h *ResourceHandlers) ListPermissions(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	defs, err := h.Permissions.Catalog(r.Context(), p)
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	out := make([]permissionDefDTO, len(defs))
	for i, d := range defs {
		out[i] = toDefDTO(d)
	}
	respondData(w, r, http.StatusOK, out)
}

// PermissionTree renders the catalog as service → resource → action → fields.
func (h *ResourceHandlers) PermissionTree(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	defs, err := h.Permissions.Catalog(r.Context(), p)
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	type fieldNode struct {
		ID    string `json:"id"`
		Field string `json:"field"`
		Code  string `json:"code"`
	}
	type actionNode struct {
		ID             string      `json:"id,omitempty"`
		Action         string      `json:"action"`
		Code           string      `json:"code"`
		IsPlatformOnly bool        `json:"is_platform_only"`
		Fields         []fieldNode `json:"fields,omitempty"`
	}
	type resourceNode struct {
		Resource string        `json:"resource"`
		Actions  []*actionNode `json:"actions"`
	}
	type serviceNode struct {
		Service   string          `json:"service"`
		Resources []*resourceNode `json:"resources"`
	}

	services := map[string]*serviceNode{}
	resources := map[string]*resourceNode{}
	actions := map[string]*actionNode{}
	var order []string

	for _, d := range defs {
		sKey := d.Service
		if _, ok := services[sKey]; !ok {
			services[sKey] = &serviceNode{Service: d.Service}
			order = append(order, sKey)
		}
		rKey := sKey + "/" + d.Resource
		if _, ok := resources[rKey]; !ok {
			resources[rKey] = &resourceNode{Resource: d.Resource}
			services[sKey].Resources = append(services[sKey].Resources, resources[rKey])
		}
		aKey := rKey + "/" + string(d.Action)
		if _, ok := actions[aKey]; !ok {
			actions[aKey] = &actionNode{Action: string(d.Action)}
			resources[rKey].Actions = append(resources[rKey].Actions, actions[aKey])
		}
		node := actions[aKey]
		if d.Field == "" {
			node.ID = d.ID
			node.Code = d.Code()
			node.IsPlatformOnly = d.IsPlatformOnly
		} else {
			node.Fields = append(node.Fields, fieldNode{ID: d.ID, Field: d.Field, Code: d.Code()})
		}
	}
	sort.Strings(order)
	out := make([]*serviceNode, 0, len(order))
	for _, k := range order {
		out = append(out, services[k])
	}
	respondData(w, r, http.StatusOK, out)
}

func (h *ResourceHandlers) GrantUserPermission(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var req grantPermissionRequest
	if _, err := decodeBody(r, &req); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	up, err := h.Permissions.GrantToUser(r.Context(), p, chi.URLParam(r, "userId"),
		app.DirectGrantCommand{
			PermissionID: req.PermissionID, Scope: req.Scope, Effect: req.Effect, Conditions: req.Conditions,
		})
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondData(w, r, http.StatusCreated, toGrantDTO(up.ID, up.Permission))
}

func (h *ResourceHandlers) RevokeUserPermission(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if err := h.Permissions.RevokeFromUser(r.Context(), p, chi.URLParam(r, "userId"), chi.URLParam(r, "permissionId")); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondNoContent(w)
}

// ─── API clients ────────────────────────────────────────────────────────────

type createAPIClientRequest struct {
	TenantID    string     `json:"tenant_id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	AllowedIPs  []string   `json:"allowed_ips"`
	Scopes      []string   `json:"scopes"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

func (h *ResourceHandlers) CreateAPIClient(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var req createAPIClientRequest
	if _, err := decodeBody(r, &req); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	created, err := h.APIClients.Create(r.Context(), p, app.CreateAPIClientCommand{
		TenantID: req.TenantID, Name: req.Name, Description: req.Description,
		AllowedIPs: req.AllowedIPs, Scopes: req.Scopes, ExpiresAt: req.ExpiresAt,
	})
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondData(w, r, http.StatusCreated, map[string]any{
		"client": toAPIClientDTO(created.Client),
		// Shown exactly once; never retrievable again.
		"client_secret": created.ClientSecret,
	})
}

func (h *ResourceHandlers) ListAPIClients(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	page := parseListPage(r)
	clients, total, err := h.APIClients.List(r.Context(), p, r.URL.Query().Get("tenant_id"), page)
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	out := make([]apiClientDTO, len(clients))
	for i, c := range clients {
		out[i] = toAPIClientDTO(c)
	}
	respondList(w, r, out, page.Page, page.PageSize, total)
}

func (h *ResourceHandlers) GetAPIClient(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	client, perms, err := h.APIClients.Get(r.Context(), p, chi.URLParam(r, "clientId"))
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	grants := make([]permissionGrantDTO, len(perms))
	for i, g := range perms {
		grants[i] = toGrantDTO(g.ID, g.Permission)
	}
	respondData(w, r, http.StatusOK, map[string]any{
		"client":      toAPIClientDTO(client),
		"permissions": grants,
	})
}

type updateAPIClientRequest struct {
	Name        *string   `json:"name"`
	Description *string   `json:"description"`
	AllowedIPs  *[]string `json:"allowed_ips"`
	Status      *string   `json:"status"`
	Version     int64     `json:"version"`
}

func (h *ResourceHandlers) UpdateAPIClient(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var req updateAPIClientRequest
	if _, err := decodeBody(r, &req); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	client, err := h.APIClients.Update(r.Context(), p, chi.URLParam(r, "clientId"),
		app.UpdateAPIClientCommand{
			Name: req.Name, Description: req.Description, AllowedIPs: req.AllowedIPs,
			Status: req.Status, Version: req.Version,
		})
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondData(w, r, http.StatusOK, toAPIClientDTO(client))
}

func (h *ResourceHandlers) DeleteAPIClient(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if err := h.APIClients.Delete(r.Context(), p, chi.URLParam(r, "clientId")); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondNoContent(w)
}

func (h *ResourceHandlers) RotateAPIClientSecret(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	rotated, err := h.APIClients.RotateSecret(r.Context(), p, chi.URLParam(r, "clientId"))
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondData(w, r, http.StatusOK, map[string]any{
		"client":        toAPIClientDTO(rotated.Client),
		"client_secret": rotated.ClientSecret,
	})
}

func (h *ResourceHandlers) GrantAPIClientPermission(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var req grantPermissionRequest
	if _, err := decodeBody(r, &req); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	grant, err := h.APIClients.GrantPermission(r.Context(), p, chi.URLParam(r, "clientId"),
		app.ClientGrantCommand{
			PermissionID: req.PermissionID, Scope: req.Scope, Effect: req.Effect, Conditions: req.Conditions,
		})
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondData(w, r, http.StatusCreated, toGrantDTO(grant.ID, grant.Permission))
}

func (h *ResourceHandlers) RevokeAPIClientPermission(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if err := h.APIClients.RevokePermission(r.Context(), p, chi.URLParam(r, "clientId"), chi.URLParam(r, "permissionId")); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondNoContent(w)
}

// ─── Audit logs ─────────────────────────────────────────────────────────────

func (h *ResourceHandlers) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	page := parseListPage(r)
	logs, total, err := h.AuditLogs.List(r.Context(), p, r.URL.Query().Get("tenant_id"), page)
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	out := make([]auditLogDTO, len(logs))
	for i, e := range logs {
		out[i] = toAuditDTO(e)
	}
	respondList(w, r, out, page.Page, page.PageSize, total)
}

func (h *ResourceHandlers) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	entry, err := h.AuditLogs.Get(r.Context(), p, chi.URLParam(r, "auditLogId"))
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondData(w, r, http.StatusOK, toAuditDTO(entry))
}
