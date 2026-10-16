// Package redis implements permission caching, rate limiting and idempotency
// on Redis. All read failures behave as cache misses (fail closed to the
// database, never open).
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
	"github.com/sr-alasgarov/wallet-gateway/internal/idempotency"
)

const keyPrefix = "wallet-gateway:v1"

// PermissionCache caches effective permission sets. Keys are versioned per
// tenant: bumping the tenant version invalidates every principal at once.
type PermissionCache struct {
	client *redis.Client
	logger *slog.Logger
}

func NewPermissionCache(client *redis.Client, logger *slog.Logger) *PermissionCache {
	return &PermissionCache{client: client, logger: logger}
}

func tenantKeyPart(tenantID string) string {
	if tenantID == "" {
		return "platform"
	}
	return tenantID
}

func (c *PermissionCache) tenantVersion(ctx context.Context, tenantID string) (int64, error) {
	key := fmt.Sprintf("%s:tenant:%s:permver", keyPrefix, tenantKeyPart(tenantID))
	v, err := c.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 1, nil
	}
	return v, err
}

func (c *PermissionCache) permKey(ctx context.Context, tenantID, principalID string) (string, error) {
	ver, err := c.tenantVersion(ctx, tenantID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:tenant:%s:v%d:principal:%s:permissions",
		keyPrefix, tenantKeyPart(tenantID), ver, principalID), nil
}

func (c *PermissionCache) Get(ctx context.Context, tenantID, principalID string) ([]domain.Permission, bool) {
	key, err := c.permKey(ctx, tenantID, principalID)
	if err != nil {
		return nil, false // Redis down => miss => authoritative DB lookup
	}
	raw, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	var perms []domain.Permission
	if err := json.Unmarshal(raw, &perms); err != nil {
		c.logger.Warn("corrupt permission cache entry", "key", key, "error", err)
		return nil, false
	}
	return perms, true
}

func (c *PermissionCache) Set(ctx context.Context, tenantID, principalID string, perms []domain.Permission, ttl time.Duration) {
	key, err := c.permKey(ctx, tenantID, principalID)
	if err != nil {
		return
	}
	raw, err := json.Marshal(perms)
	if err != nil {
		return
	}
	if err := c.client.Set(ctx, key, raw, ttl).Err(); err != nil {
		c.logger.Debug("permission cache set failed", "error", err)
	}
}

func (c *PermissionCache) InvalidatePrincipal(ctx context.Context, tenantID, principalID string) {
	key, err := c.permKey(ctx, tenantID, principalID)
	if err != nil {
		return
	}
	if err := c.client.Del(ctx, key).Err(); err != nil {
		c.logger.Debug("permission cache invalidate failed", "error", err)
	}
}

// InvalidateTenant bumps the tenant version; all cached entries under the old
// version become unreachable and expire by TTL.
func (c *PermissionCache) InvalidateTenant(ctx context.Context, tenantID string) {
	key := fmt.Sprintf("%s:tenant:%s:permver", keyPrefix, tenantKeyPart(tenantID))
	if err := c.client.Incr(ctx, key).Err(); err != nil {
		c.logger.Warn("tenant cache version bump failed", "tenant", tenantID, "error", err)
	}
}

// ─── Rate limiter (fixed window) ────────────────────────────────────────────

type RateLimiter struct {
	client *redis.Client
}

func NewRateLimiter(client *redis.Client) *RateLimiter { return &RateLimiter{client: client} }

// Allow implements a fixed-window counter. On Redis errors it returns an
// error; callers decide (login limiter fails closed, generic limiter open).
func (l *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	bucket := time.Now().Unix() / int64(window.Seconds())
	k := fmt.Sprintf("%s:ratelimit:%s:%d", keyPrefix, key, bucket)
	pipe := l.client.TxPipeline()
	incr := pipe.Incr(ctx, k)
	pipe.Expire(ctx, k, window+time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return incr.Val() <= int64(limit), nil
}

// ─── Idempotency store ──────────────────────────────────────────────────────

type IdempotencyStore struct {
	client *redis.Client
}

func NewIdempotencyStore(client *redis.Client) *IdempotencyStore {
	return &IdempotencyStore{client: client}
}

func idemKey(key string) string { return keyPrefix + ":idempotency:" + key }

func (s *IdempotencyStore) Begin(ctx context.Context, key, requestHash string, ttl time.Duration) (*idempotency.Record, bool, error) {
	rec := idempotency.Record{RequestHash: requestHash, Completed: false}
	raw, _ := json.Marshal(rec)
	ok, err := s.client.SetNX(ctx, idemKey(key), raw, ttl).Result()
	if err != nil {
		return nil, false, err
	}
	if ok {
		return nil, true, nil
	}
	existing, err := s.client.Get(ctx, idemKey(key)).Bytes()
	if err != nil {
		return nil, false, err
	}
	var existingRec idempotency.Record
	if err := json.Unmarshal(existing, &existingRec); err != nil {
		return nil, false, err
	}
	return &existingRec, false, nil
}

func (s *IdempotencyStore) Complete(ctx context.Context, key string, rec idempotency.Record, ttl time.Duration) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, idemKey(key), raw, ttl).Err()
}

func (s *IdempotencyStore) Release(ctx context.Context, key string) error {
	return s.client.Del(ctx, idemKey(key)).Err()
}
