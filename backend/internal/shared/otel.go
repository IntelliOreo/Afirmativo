// OTel initialization for GCP Cloud Trace and Cloud Monitoring.
// When disabled (default), uses noop providers with zero overhead.
package shared

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	gcpmetric "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	gcptrace "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// OTelConfig holds settings for OpenTelemetry initialization.
type OTelConfig struct {
	Enabled           bool
	GCPProjectID      string
	ServiceName       string
	ServiceInstanceID string
}

// OTelShutdown is returned by InitOTel and must be called on process exit.
type OTelShutdown func(ctx context.Context) error

// InitOTel sets up GCP trace and metric exporters when enabled.
// Returns a shutdown function that flushes pending data.
// When disabled, returns a no-op shutdown and leaves default noop providers.
func InitOTel(ctx context.Context, cfg OTelConfig) (OTelShutdown, error) {
	if !cfg.Enabled {
		slog.Info("otel disabled, using noop providers")
		return func(ctx context.Context) error { return nil }, nil
	}

	if cfg.GCPProjectID == "" {
		return nil, fmt.Errorf("GCP_PROJECT_ID is required when OTEL_ENABLED=true")
	}

	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = "afirmativo-backend"
	}
	serviceInstanceID := strings.TrimSpace(cfg.ServiceInstanceID)

	attributes := []attribute.KeyValue{
		attribute.String("service.name", serviceName),
	}
	if serviceInstanceID != "" {
		attributes = append(attributes, attribute.String("service.instance.id", serviceInstanceID))
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			attributes...,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create otel resource: %w", err)
	}

	// Trace exporter → Cloud Trace.
	traceExporter, err := gcptrace.New(gcptrace.WithProjectID(cfg.GCPProjectID))
	if err != nil {
		return nil, fmt.Errorf("create gcp trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	// Metric exporter → Cloud Monitoring.
	metricExporter, err := gcpmetric.New(gcpmetric.WithProjectID(cfg.GCPProjectID))
	if err != nil {
		// Non-fatal: tracing still works without metrics.
		slog.Error("failed to create gcp metric exporter, metrics disabled", "error", err)
		return func(ctx context.Context) error {
			return tp.Shutdown(ctx)
		}, nil
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	slog.Info("otel initialized",
		"project_id", cfg.GCPProjectID,
		"service_name", serviceName,
		"service_instance_id", serviceInstanceID,
	)

	return func(ctx context.Context) error {
		tpErr := tp.Shutdown(ctx)
		mpErr := mp.Shutdown(ctx)
		if tpErr != nil {
			return tpErr
		}
		return mpErr
	}, nil
}
