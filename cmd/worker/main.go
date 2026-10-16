// Command worker drains the transactional outbox to Kafka.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/sr-alasgarov/wallet-gateway/internal/app"
	"github.com/sr-alasgarov/wallet-gateway/internal/db"
	"github.com/sr-alasgarov/wallet-gateway/internal/kafka"
	"github.com/sr-alasgarov/wallet-gateway/internal/observability"
	"github.com/sr-alasgarov/wallet-gateway/internal/postgres"
	"github.com/sr-alasgarov/wallet-gateway/internal/setup"
)

func main() {
	logger := observability.NewLogger(os.Getenv("LOG_LEVEL"), os.Getenv("LOG_FORMAT"), "outbox-worker")

	cfg, err := setup.LoadConfig()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := observability.SetupTracing(ctx, "outbox-worker", cfg.OTLPEndpoint)
	if err != nil {
		logger.Warn("tracing disabled", "error", err)
		shutdownTracing = func(context.Context) error { return nil }
	}
	defer func() { _ = shutdownTracing(context.Background()) }()

	pool, err := db.NewPool(ctx, db.Config{URL: cfg.DatabaseAppURL, MaxConns: 5, MinConns: 1})
	if err != nil {
		logger.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	publisher := kafka.NewPublisher(cfg.KafkaBrokers, cfg.KafkaTopicPrefix)
	defer func() { _ = publisher.Close() }()

	metrics := observability.NewMetrics("outbox_worker")
	metricsSrv := observability.ServeMetrics(cfg.MetricsAddr, metrics)
	defer func() { _ = metricsSrv.Shutdown(context.Background()) }()

	worker := &app.OutboxWorker{
		Tx:           postgres.NewTxManager(pool),
		Outbox:       postgres.NewOutboxRepo(),
		Sink:         publisher,
		Logger:       logger,
		PollInterval: cfg.OutboxPollInterval,
		BatchSize:    cfg.OutboxBatchSize,
		MaxAttempts:  cfg.OutboxMaxAttempts,
		DLQTopic:     cfg.KafkaDLQTopic,
		OnPublished:  func() { metrics.OutboxPublished.Inc() },
		OnFailed:     func() { metrics.OutboxFailed.Inc() },
	}
	worker.Run(ctx)
}
