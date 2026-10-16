// Package idempotency defines the idempotency-key contract used by write
// endpoints. Storage lives in an adapter (Redis); this package holds the
// port and request fingerprinting so duplicate keys with different payloads
// are rejected.
package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Record is the stored outcome of a completed idempotent request.
type Record struct {
	RequestHash string `json:"request_hash"`
	StatusCode  int    `json:"status_code"`
	Body        []byte `json:"body"`
	ContentType string `json:"content_type"`
	Completed   bool   `json:"completed"`
}

// Store persists idempotency records. Implementations must be safe for
// concurrent use across service instances.
type Store interface {
	// Begin atomically claims the key. Returns (existing, claimed, error):
	// claimed=true means the caller owns the key and must Complete or Release.
	Begin(ctx context.Context, key string, requestHash string, ttl time.Duration) (*Record, bool, error)
	// Complete stores the final response for replay.
	Complete(ctx context.Context, key string, rec Record, ttl time.Duration) error
	// Release frees a claimed key after a failure so the client may retry.
	Release(ctx context.Context, key string) error
}

// FingerprintRequest hashes method+path+body to detect key reuse with a
// different payload.
func FingerprintRequest(method, path string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte{0})
	h.Write([]byte(path))
	h.Write([]byte{0})
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}
