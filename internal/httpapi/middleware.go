package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/sr-alasgarov/wallet-gateway/internal/app"
	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/auth"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
	"github.com/sr-alasgarov/wallet-gateway/internal/idempotency"
	"github.com/sr-alasgarov/wallet-gateway/internal/observability"
	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
)

type ctxKeyRequestID struct{}
type ctxKeyPrincipal struct{}

func requestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRequestID{}).(string); ok {
		return v
	}
	return ""
}

func principalFrom(ctx context.Context) (domain.Principal, bool) {
	p, ok := ctx.Value(ctxKeyPrincipal{}).(domain.Principal)
	return p, ok
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RequestContext assigns request/correlation ids and builds app.Meta.
func RequestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-Id")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		correlationID := r.Header.Get("X-Correlation-Id")
		if correlationID == "" {
			correlationID = requestID
		}
		ctx := context.WithValue(r.Context(), ctxKeyRequestID{}, requestID)
		ctx = app.WithMeta(ctx, app.Meta{
			IP:            clientIP(r),
			UserAgent:     r.UserAgent(),
			RequestID:     requestID,
			CorrelationID: correlationID,
		})
		w.Header().Set("X-Request-Id", requestID)
		w.Header().Set("X-Correlation-Id", correlationID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SecurityHeaders sets conservative security headers on every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// CORS implements a strict allow-list.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range allowedOrigins {
		allowed[strings.TrimRight(strings.TrimSpace(o), "/")] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowed[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
				// Every custom header the portal sends must be listed here, or
				// the browser blocks the request at preflight. X-Auth-Mode is
				// what opts a browser into HttpOnly refresh cookies.
				w.Header().Set("Access-Control-Allow-Headers",
					"Authorization, Content-Type, X-Request-Id, X-Correlation-Id, Idempotency-Key, X-Auth-Mode")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// BodyLimit caps request body size.
func BodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// Timeout cancels handlers exceeding d.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Recover converts panics into 500 problem responses.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered", "panic", rec, "path", r.URL.Path)
					respondError(w, r, apperr.Internal(nil), logger)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// AccessLog logs structured request/response lines and records metrics.
func AccessLog(logger *slog.Logger, metrics *observability.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			dur := time.Since(start)
			route := routePattern(r)
			logger.Info("http_request",
				"method", r.Method, "path", r.URL.Path, "status", sw.status,
				"duration_ms", dur.Milliseconds(), "request_id", requestIDFrom(r.Context()),
				"ip", clientIP(r))
			if metrics != nil {
				statusLabel := statusClass(sw.status)
				metrics.RequestsTotal.WithLabelValues(r.Method, route, statusLabel).Inc()
				metrics.RequestDuration.WithLabelValues(r.Method, route, statusLabel).Observe(dur.Seconds())
			}
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func statusClass(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	default:
		return "2xx"
	}
}

// RateLimit applies a per-IP fixed window limit. Redis failures fail OPEN for
// general traffic (availability) — but the dedicated login limiter in the
// auth handler fails CLOSED.
func RateLimit(limiter ports.RateLimiter, rps int, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ok, err := limiter.Allow(r.Context(), "ip:"+clientIP(r), rps, time.Second)
			if err != nil {
				logger.Debug("rate limiter unavailable, allowing", "error", err)
				next.ServeHTTP(w, r)
				return
			}
			if !ok {
				respondError(w, r, apperr.RateLimited(), logger)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Authenticate parses the bearer token and attaches the Principal.
func Authenticate(jwtMgr *auth.JWTManager, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" || !strings.HasPrefix(header, "Bearer ") {
				respondError(w, r, apperr.Unauthorized("Missing bearer token."), logger)
				return
			}
			claims, err := jwtMgr.Parse(strings.TrimPrefix(header, "Bearer "))
			if err != nil {
				respondError(w, r, apperr.Unauthorized("Invalid or expired token."), logger)
				return
			}
			principal := domain.Principal{
				ID:        claims.Subject,
				Kind:      domain.PrincipalKind(claims.PrincipalType),
				TenantID:  claims.TenantID,
				SessionID: claims.SessionID,
				ClientID:  claims.ClientID,
				Scopes:    claims.Scopes,
			}
			ctx := context.WithValue(r.Context(), ctxKeyPrincipal{}, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Idempotency replays completed responses for repeated Idempotency-Key
// headers and rejects key reuse with different payloads.
func Idempotency(store idempotency.Store, logger *slog.Logger) func(http.Handler) http.Handler {
	const ttl = 24 * time.Hour
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost && r.Method != http.MethodPatch {
				next.ServeHTTP(w, r)
				return
			}
			key := r.Header.Get("Idempotency-Key")
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}
			principal, _ := principalFrom(r.Context())
			body, err := io.ReadAll(r.Body)
			if err != nil {
				respondError(w, r, apperr.BadRequest("Cannot read request body."), logger)
				return
			}
			r.Body = io.NopCloser(strings.NewReader(string(body)))
			fingerprint := idempotency.FingerprintRequest(r.Method, r.URL.Path+"|"+principal.ID, body)
			scopedKey := principal.ID + ":" + key

			existing, claimed, err := store.Begin(r.Context(), scopedKey, fingerprint, ttl)
			if err != nil {
				// Redis down: proceed without idempotency (writes remain safe
				// via DB constraints), but log loudly.
				logger.Warn("idempotency store unavailable", "error", err)
				next.ServeHTTP(w, r)
				return
			}
			if !claimed {
				if existing.RequestHash != fingerprint {
					respondError(w, r, apperr.IdempotencyConflict(), logger)
					return
				}
				if existing.Completed {
					w.Header().Set("Content-Type", existing.ContentType)
					w.Header().Set("Idempotency-Replayed", "true")
					w.WriteHeader(existing.StatusCode)
					_, _ = w.Write(existing.Body)
					return
				}
				respondError(w, r, apperr.Conflict("A request with this idempotency key is still in progress."), logger)
				return
			}
			rec := &recordingWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			if rec.status < 500 {
				_ = store.Complete(r.Context(), scopedKey, idempotency.Record{
					RequestHash: fingerprint,
					StatusCode:  rec.status,
					Body:        rec.body,
					ContentType: rec.Header().Get("Content-Type"),
					Completed:   true,
				}, ttl)
			} else {
				_ = store.Release(r.Context(), scopedKey)
			}
		})
	}
}

type recordingWriter struct {
	http.ResponseWriter
	status int
	body   []byte
}

func (w *recordingWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *recordingWriter) Write(b []byte) (int, error) {
	w.body = append(w.body, b...)
	return w.ResponseWriter.Write(b)
}
