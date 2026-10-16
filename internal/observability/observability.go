// Package observability wires structured logging, Prometheus metrics and
// OpenTelemetry tracing for all services.
package observability

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// NewLogger builds a structured slog logger (json or text).
func NewLogger(level, format, service string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var handler slog.Handler
	if format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(handler).With("service", service)
}

// SetupTracing configures the global OTLP/HTTP trace exporter. Returns a
// shutdown function. If endpoint is empty, tracing is a no-op.
func SetupTracing(ctx context.Context, serviceName, endpoint string) (func(context.Context) error, error) {
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(endpoint+"/v1/traces"),
		otlptracehttp.WithTimeout(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}
	res, err := sdkresource.Merge(sdkresource.Default(),
		sdkresource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(serviceName)))
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))
	return tp.Shutdown, nil
}

// Metrics holds the HTTP server metrics for a service.
type Metrics struct {
	Registry        *prometheus.Registry
	RequestDuration *prometheus.HistogramVec
	RequestsTotal   *prometheus.CounterVec
	AuthzDecisions  *prometheus.CounterVec
	OutboxPublished prometheus.Counter
	OutboxFailed    prometheus.Counter
}

func NewMetrics(service string) *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector())
	factory := promauto.With(reg)
	return &Metrics{
		Registry: reg,
		RequestDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "wallet_gateway", Subsystem: service, Name: "http_request_duration_seconds",
			Help: "HTTP request latency", Buckets: prometheus.DefBuckets,
		}, []string{"method", "route", "status"}),
		RequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "wallet_gateway", Subsystem: service, Name: "http_requests_total",
			Help: "HTTP requests",
		}, []string{"method", "route", "status"}),
		AuthzDecisions: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "wallet_gateway", Subsystem: service, Name: "authz_decisions_total",
			Help: "Authorization decisions",
		}, []string{"decision", "reason"}),
		OutboxPublished: factory.NewCounter(prometheus.CounterOpts{
			Namespace: "wallet_gateway", Subsystem: service, Name: "outbox_published_total",
			Help: "Outbox events published to Kafka",
		}),
		OutboxFailed: factory.NewCounter(prometheus.CounterOpts{
			Namespace: "wallet_gateway", Subsystem: service, Name: "outbox_failed_total",
			Help: "Outbox publish failures",
		}),
	}
}

// ServeMetrics starts a metrics HTTP endpoint on addr.
func ServeMetrics(addr string, m *Metrics) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}))
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.ListenAndServe() }()
	return srv
}
