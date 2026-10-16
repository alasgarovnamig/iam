// Package infrastructure holds process-level configuration and wiring
// helpers (environment parsing, container assembly).
package setup

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the full service configuration, sourced from the environment.
type Config struct {
	DatabaseURL    string
	DatabaseAppURL string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	KafkaBrokers     []string
	KafkaTopicPrefix string
	KafkaDLQTopic    string

	OPAURL         string
	OpenFGAURL     string
	OpenFGAStoreID string
	OpenFGAModelID string

	JWTSigningKey  string
	JWTIssuer      string
	JWTAudience    string
	AccessTokenTTL time.Duration
	RefreshTTL     time.Duration
	ClientTokenTTL time.Duration

	BootstrapAdminEmail    string
	BootstrapAdminPassword string

	HTTPAddr       string
	CORSOrigins    []string
	RequestTimeout time.Duration
	MaxBodyBytes   int64
	RateLimitRPS   int
	LoginPerMin    int

	LogLevel     string
	LogFormat    string
	OTLPEndpoint string
	ServiceName  string
	MetricsAddr  string

	OutboxPollInterval time.Duration
	OutboxBatchSize    int
	OutboxMaxAttempts  int
}

// LoadConfig reads configuration from the environment and validates it.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		DatabaseURL:    env("DATABASE_URL", ""),
		DatabaseAppURL: env("DATABASE_APP_URL", ""),

		RedisAddr:     env("REDIS_ADDR", "localhost:6379"),
		RedisPassword: env("REDIS_PASSWORD", ""),
		RedisDB:       envInt("REDIS_DB", 0),

		KafkaBrokers:     strings.Split(env("KAFKA_BROKERS", "localhost:9092"), ","),
		KafkaTopicPrefix: env("KAFKA_TOPIC_PREFIX", "wallet-gateway"),
		KafkaDLQTopic:    env("KAFKA_DLQ_TOPIC", "wallet-gateway.dlq"),

		OPAURL:         env("OPA_URL", "http://localhost:8181"),
		OpenFGAURL:     env("OPENFGA_URL", "http://localhost:8080"),
		OpenFGAStoreID: env("OPENFGA_STORE_ID", ""),
		OpenFGAModelID: env("OPENFGA_MODEL_ID", ""),

		JWTSigningKey:  env("JWT_SIGNING_KEY", ""),
		JWTIssuer:      env("JWT_ISSUER", "wallet-gateway"),
		JWTAudience:    env("JWT_AUDIENCE", "wallet-gateway-api"),
		AccessTokenTTL: envDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTTL:     envDuration("REFRESH_TOKEN_TTL", 720*time.Hour),
		ClientTokenTTL: envDuration("CLIENT_TOKEN_TTL", 30*time.Minute),

		BootstrapAdminEmail:    env("BOOTSTRAP_ADMIN_EMAIL", ""),
		BootstrapAdminPassword: env("BOOTSTRAP_ADMIN_PASSWORD", ""),

		HTTPAddr:       env("HTTP_ADDR", ":8090"),
		CORSOrigins:    splitAndTrim(env("CORS_ALLOWED_ORIGINS", "http://localhost:5173")),
		RequestTimeout: envDuration("REQUEST_TIMEOUT", 30*time.Second),
		MaxBodyBytes:   int64(envInt("MAX_BODY_BYTES", 1<<20)),
		RateLimitRPS:   envInt("RATE_LIMIT_RPS", 50),
		LoginPerMin:    envInt("LOGIN_RATE_LIMIT_PER_MIN", 10),

		LogLevel:     env("LOG_LEVEL", "info"),
		LogFormat:    env("LOG_FORMAT", "json"),
		OTLPEndpoint: env("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		ServiceName:  env("OTEL_SERVICE_NAME", "user-management-service"),
		MetricsAddr:  env("METRICS_ADDR", ":9091"),

		OutboxPollInterval: envDuration("OUTBOX_POLL_INTERVAL", time.Second),
		OutboxBatchSize:    envInt("OUTBOX_BATCH_SIZE", 100),
		OutboxMaxAttempts:  envInt("OUTBOX_MAX_ATTEMPTS", 10),
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.DatabaseAppURL == "" {
		// Fall back to the owner connection; RLS then relies on the owner not
		// bypassing policies, which it does — warn the operator loudly.
		cfg.DatabaseAppURL = cfg.DatabaseURL
	}
	if cfg.JWTSigningKey == "" {
		return nil, fmt.Errorf("JWT_SIGNING_KEY is required")
	}
	if len(cfg.JWTSigningKey) < 32 {
		return nil, fmt.Errorf("JWT_SIGNING_KEY must be at least 32 characters")
	}
	// OPENFGA_STORE_ID / OPENFGA_MODEL_ID are optional: when unset the service
	// discovers (or creates) the store and applies the embedded model at
	// start-up. Pin them in production for deterministic deployments.
	return cfg, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
