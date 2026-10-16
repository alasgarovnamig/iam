package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/sr-alasgarov/wallet-gateway/internal/auth"
	"github.com/sr-alasgarov/wallet-gateway/internal/idempotency"
	"github.com/sr-alasgarov/wallet-gateway/internal/observability"
	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
)

// RouterConfig wires the HTTP surface.
type RouterConfig struct {
	AuthHandlers     *AuthHandlers
	ResourceHandlers *ResourceHandlers
	JWT              *auth.JWTManager
	RateLimiter      ports.RateLimiter
	IdempotencyStore idempotency.Store
	Metrics          *observability.Metrics
	Logger           *slog.Logger
	CORSOrigins      []string
	MaxBodyBytes     int64
	RequestTimeout   time.Duration
	RateLimitRPS     int
	HealthCheck      func(ctx context.Context) map[string]string
}

func routePattern(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		if p := rc.RoutePattern(); p != "" {
			return p
		}
	}
	return "unmatched"
}

// NewRouter builds the chi router with the full middleware chain. Order:
// recovery → request context → security → CORS → limits → logging → auth.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	r.Use(Recover(cfg.Logger))
	r.Use(RequestContext)
	r.Use(SecurityHeaders)
	r.Use(CORS(cfg.CORSOrigins))
	r.Use(BodyLimit(cfg.MaxBodyBytes))
	r.Use(Timeout(cfg.RequestTimeout))
	r.Use(AccessLog(cfg.Logger, cfg.Metrics))
	r.Use(RateLimit(cfg.RateLimiter, cfg.RateLimitRPS, cfg.Logger))

	// Liveness/readiness (no auth).
	r.Get("/healthz", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		status := cfg.HealthCheck(req.Context())
		code := http.StatusOK
		for _, v := range status {
			// "degraded" (e.g. Redis) keeps the instance in rotation; only a
			// hard "unavailable" dependency fails readiness.
			if v == "unavailable" {
				code = http.StatusServiceUnavailable
			}
		}
		writeJSON(w, code, status)
	})

	r.Route("/api/v1", func(api chi.Router) {
		// Public endpoints.
		api.Post("/auth/login", cfg.AuthHandlers.Login)
		api.Post("/auth/refresh", cfg.AuthHandlers.Refresh)
		api.Post("/oauth/token", cfg.AuthHandlers.OAuthToken)

		// Authenticated endpoints.
		api.Group(func(priv chi.Router) {
			priv.Use(Authenticate(cfg.JWT, cfg.Logger))
			priv.Use(Idempotency(cfg.IdempotencyStore, cfg.Logger))

			priv.Post("/auth/logout", cfg.AuthHandlers.Logout)
			priv.Post("/auth/logout-all", cfg.AuthHandlers.LogoutAll)
			priv.Post("/auth/change-password", cfg.AuthHandlers.ChangePassword)
			priv.Get("/auth/me", cfg.AuthHandlers.Me)
			priv.Get("/auth/sessions", cfg.AuthHandlers.Sessions)
			priv.Delete("/auth/sessions/{sessionId}", cfg.AuthHandlers.RevokeSession)

			h := cfg.ResourceHandlers
			priv.Post("/tenants", h.CreateTenant)
			priv.Post("/tenants/search", h.SearchTenants)
			priv.Get("/tenants", h.ListTenants)
			priv.Get("/tenants/{tenantId}", h.GetTenant)
			priv.Patch("/tenants/{tenantId}", h.UpdateTenant)
			priv.Delete("/tenants/{tenantId}", h.DeleteTenant)

			priv.Post("/users", h.CreateUser)
			priv.Post("/users/search", h.SearchUsers)
			priv.Get("/users", h.ListUsers)
			priv.Get("/users/{userId}", h.GetUser)
			priv.Patch("/users/{userId}", h.UpdateUser)
			priv.Delete("/users/{userId}", h.DeleteUser)
			priv.Post("/users/{userId}/activate", h.ActivateUser())
			priv.Post("/users/{userId}/suspend", h.SuspendUser())
			priv.Post("/users/{userId}/unlock", h.UnlockUser())
			priv.Get("/users/{userId}/effective-permissions", h.UserEffectivePermissions)
			priv.Post("/users/{userId}/roles", h.AssignRoleToUser)
			priv.Delete("/users/{userId}/roles/{roleId}", h.RevokeRoleFromUser)
			priv.Post("/users/{userId}/permissions", h.GrantUserPermission)
			priv.Delete("/users/{userId}/permissions/{permissionId}", h.RevokeUserPermission)

			priv.Post("/roles", h.CreateRole)
			priv.Get("/roles", h.ListRoles)
			priv.Get("/roles/{roleId}", h.GetRole)
			priv.Patch("/roles/{roleId}", h.UpdateRole)
			priv.Delete("/roles/{roleId}", h.DeleteRole)
			priv.Post("/roles/{roleId}/permissions", h.AddRolePermission)
			priv.Delete("/roles/{roleId}/permissions/{permissionId}", h.RemoveRolePermission)

			priv.Get("/permissions", h.ListPermissions)
			priv.Get("/permissions/tree", h.PermissionTree)

			priv.Post("/api-clients", h.CreateAPIClient)
			priv.Post("/api-clients/search", h.SearchAPIClients)
			priv.Get("/api-clients", h.ListAPIClients)
			priv.Get("/api-clients/{clientId}", h.GetAPIClient)
			priv.Patch("/api-clients/{clientId}", h.UpdateAPIClient)
			priv.Delete("/api-clients/{clientId}", h.DeleteAPIClient)
			priv.Post("/api-clients/{clientId}/rotate-secret", h.RotateAPIClientSecret)
			priv.Post("/api-clients/{clientId}/permissions", h.GrantAPIClientPermission)
			priv.Delete("/api-clients/{clientId}/permissions/{permissionId}", h.RevokeAPIClientPermission)

			priv.Get("/audit-logs", h.ListAuditLogs)
			priv.Get("/audit-logs/{auditLogId}", h.GetAuditLog)
		})
	})

	return r
}
