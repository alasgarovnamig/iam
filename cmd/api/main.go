// Command api runs the user-management HTTP API. On start it seeds the
// bootstrap administrator (idempotent) and serves /api/v1.
package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sr-alasgarov/wallet-gateway/internal/httpapi"
	"github.com/sr-alasgarov/wallet-gateway/internal/observability"
	"github.com/sr-alasgarov/wallet-gateway/internal/seed"
	"github.com/sr-alasgarov/wallet-gateway/internal/setup"
)

func main() {
	seedOnly := flag.Bool("seed-only", false, "run the bootstrap seed and exit")
	flag.Parse()

	cfg, err := setup.LoadConfig()
	logger := observability.NewLogger(os.Getenv("LOG_LEVEL"), os.Getenv("LOG_FORMAT"), "user-management-api")
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := observability.SetupTracing(ctx, cfg.ServiceName, cfg.OTLPEndpoint)
	if err != nil {
		logger.Warn("tracing disabled", "error", err)
		shutdownTracing = func(context.Context) error { return nil }
	}

	container, err := setup.Build(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to build container", "error", err)
		os.Exit(1)
	}
	defer container.Close()

	// Idempotent bootstrap seed: creates the platform super admin if absent.
	seeder := &seed.Seeder{
		Tx:          container.Deps.Tx,
		Users:       container.Deps.Users,
		Credentials: container.Deps.Credentials,
		Roles:       container.Deps.Roles,
		FGA:         container.Deps.FGA,
		Hasher:      container.Deps.Hasher,
		Logger:      logger,
	}
	seedCtx, cancelSeed := context.WithTimeout(ctx, 60*time.Second)
	err = seeder.Run(seedCtx, seed.BootstrapAdmin{
		Email:    cfg.BootstrapAdminEmail,
		Password: cfg.BootstrapAdminPassword,
	})
	cancelSeed()
	if err != nil {
		logger.Error("bootstrap seed failed", "error", err)
		os.Exit(1)
	}
	if *seedOnly {
		logger.Info("seed complete")
		return
	}

	metricsSrv := observability.ServeMetrics(cfg.MetricsAddr, container.Metrics)
	logger.Info("metrics endpoint listening", "addr", cfg.MetricsAddr)

	router := httpapi.NewRouter(httpapi.RouterConfig{
		AuthHandlers: &httpapi.AuthHandlers{
			Auth:        container.Auth,
			OAuth:       container.OAuth,
			Limiter:     container.RateLimiter,
			LoginPerMin: cfg.LoginPerMin,
			RefreshTTL:  cfg.RefreshTTL,
			Logger:      logger,
		},
		ResourceHandlers: &httpapi.ResourceHandlers{
			Tenants:     container.Tenants,
			Users:       container.Users,
			Roles:       container.Roles,
			Permissions: container.Permissions,
			APIClients:  container.APIClients,
			AuditLogs:   container.Audit,
			Logger:      logger,
		},
		JWT:              container.Deps.JWT,
		RateLimiter:      container.RateLimiter,
		IdempotencyStore: container.IdempotencyStore,
		Metrics:          container.Metrics,
		Logger:           logger,
		CORSOrigins:      cfg.CORSOrigins,
		MaxBodyBytes:     cfg.MaxBodyBytes,
		RequestTimeout:   cfg.RequestTimeout,
		RateLimitRPS:     cfg.RateLimitRPS,
		HealthCheck:      container.HealthCheck,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		logger.Info("user-management API listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
	_ = metricsSrv.Shutdown(shutdownCtx)
	_ = shutdownTracing(shutdownCtx)
	logger.Info("stopped")
}
