package setup

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/sr-alasgarov/wallet-gateway/internal/app"
	"github.com/sr-alasgarov/wallet-gateway/internal/auth"
	"github.com/sr-alasgarov/wallet-gateway/internal/cache"
	"github.com/sr-alasgarov/wallet-gateway/internal/db"
	"github.com/sr-alasgarov/wallet-gateway/internal/idempotency"
	"github.com/sr-alasgarov/wallet-gateway/internal/observability"
	"github.com/sr-alasgarov/wallet-gateway/internal/opa"
	"github.com/sr-alasgarov/wallet-gateway/internal/openfga"
	"github.com/sr-alasgarov/wallet-gateway/internal/postgres"
)

// Container holds constructed dependencies and their shutdown hooks.
type Container struct {
	Config *Config
	Logger *slog.Logger
	Pool   *pgxpool.Pool
	Redis  *redis.Client

	Deps        *app.Deps
	Authorizer  *app.Authorizer
	Auth        *app.AuthService
	OAuth       *app.OAuthService
	Tenants     *app.TenantService
	Users       *app.UserService
	Roles       *app.RoleService
	Permissions *app.PermissionService
	APIClients  *app.APIClientService
	Audit       *app.AuditService

	RateLimiter      *cache.RateLimiter
	IdempotencyStore idempotency.Store
	FGA              *openfga.Client
	OPA              *opa.Client
	Metrics          *observability.Metrics
}

// Build wires every adapter and service. The application connects with the
// RLS-enforced role (DatabaseAppURL).
func Build(ctx context.Context, cfg *Config, logger *slog.Logger) (*Container, error) {
	pool, err := db.NewPool(ctx, db.Config{
		URL:             cfg.DatabaseAppURL,
		MaxConns:        20,
		MinConns:        2,
		MaxConnLifetime: time.Hour,
	})
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	// Redis is not fatal at startup: caching degrades to direct DB reads.
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := redisClient.Ping(pingCtx).Err(); err != nil {
		logger.Warn("redis not reachable at startup; caching/rate limiting degraded", "error", err)
	}

	txManager := postgres.NewTxManager(pool)

	// OpenFGA: wait for readiness, then resolve the store and apply the
	// embedded authorization model. Without this the service cannot make a
	// single authorization decision, so a failure here is fatal.
	if err := openfga.WaitReady(ctx, cfg.OpenFGAURL, 60*time.Second); err != nil {
		pool.Close()
		return nil, fmt.Errorf("openfga not ready: %w", err)
	}
	storeID, modelID, err := openfga.Bootstrap(ctx, cfg.OpenFGAURL, cfg.OpenFGAStoreID, cfg.OpenFGAModelID)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("bootstrap openfga model: %w", err)
	}
	logger.Info("openfga ready", "store_id", storeID, "model_id", modelID)
	fga := openfga.NewClient(cfg.OpenFGAURL, storeID, modelID)
	opaClient := opa.NewClient(cfg.OPAURL)
	metrics := observability.NewMetrics("user_management")

	deps := &app.Deps{
		Tx:          txManager,
		Tenants:     postgres.NewTenantRepo(),
		Users:       postgres.NewUserRepo(),
		Credentials: postgres.NewCredentialRepo(),
		Sessions:    postgres.NewSessionRepo(),
		Roles:       postgres.NewRoleRepo(),
		Permissions: postgres.NewPermissionRepo(),
		APIClients:  postgres.NewAPIClientRepo(),
		Audit:       postgres.NewAuditRepo(),
		Outbox:      postgres.NewOutboxRepo(),
		Cache:       cache.NewPermissionCache(redisClient, logger),
		FGA:         fga,
		OPA:         opaClient,
		Hasher:      auth.NewPasswordHasher(auth.DefaultArgon2Params()),
		JWT: auth.NewJWTManager(auth.JWTConfig{
			SigningKey:     []byte(cfg.JWTSigningKey),
			Issuer:         cfg.JWTIssuer,
			Audience:       cfg.JWTAudience,
			AccessTokenTTL: cfg.AccessTokenTTL,
			ClientTokenTTL: cfg.ClientTokenTTL,
		}),
		Logger: logger,
		Now:    func() time.Time { return time.Now().UTC() },
	}
	authorizer := app.NewAuthorizer(deps)

	return &Container{
		Config:           cfg,
		Logger:           logger,
		Pool:             pool,
		Redis:            redisClient,
		Deps:             deps,
		Authorizer:       authorizer,
		Auth:             app.NewAuthService(deps, authorizer),
		OAuth:            app.NewOAuthService(deps),
		Tenants:          app.NewTenantService(deps, authorizer),
		Users:            app.NewUserService(deps, authorizer),
		Roles:            app.NewRoleService(deps, authorizer),
		Permissions:      app.NewPermissionService(deps, authorizer),
		APIClients:       app.NewAPIClientService(deps, authorizer),
		Audit:            app.NewAuditService(deps, authorizer),
		RateLimiter:      cache.NewRateLimiter(redisClient),
		IdempotencyStore: cache.NewIdempotencyStore(redisClient),
		FGA:              fga,
		OPA:              opaClient,
		Metrics:          metrics,
	}, nil
}

// Close releases pooled resources.
func (c *Container) Close() {
	if c.Pool != nil {
		c.Pool.Close()
	}
	if c.Redis != nil {
		_ = c.Redis.Close()
	}
}

// HealthCheck reports dependency status for /readyz.
func (c *Container) HealthCheck(ctx context.Context) map[string]string {
	status := map[string]string{"database": "ok", "redis": "ok", "opa": "ok", "openfga": "ok"}
	if err := c.Pool.Ping(ctx); err != nil {
		status["database"] = "unavailable"
	}
	if err := c.Redis.Ping(ctx).Err(); err != nil {
		status["redis"] = "degraded"
	}
	if err := c.OPA.Healthy(ctx); err != nil {
		status["opa"] = "unavailable"
	}
	if err := c.FGA.Healthy(ctx); err != nil {
		status["openfga"] = "unavailable"
	}
	return status
}
