package otel

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config holds OpenTelemetry configuration
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	OTLPEndpoint   string
	Enabled        bool
}

// ConfigFromEnv creates Config from environment variables
func ConfigFromEnv() Config {
	enabled := getEnv("OTEL_ENABLED", "true") == "true"
	return Config{
		ServiceName:    getEnv("OTEL_SERVICE_NAME", "rag-orchestrator"),
		ServiceVersion: getEnv("SERVICE_VERSION", "0.0.0"),
		Environment:    getEnv("DEPLOYMENT_ENV", "development"),
		OTLPEndpoint:   getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318"),
		Enabled:        enabled,
	}
}

// ShutdownFunc is a function to shutdown providers
type ShutdownFunc func(context.Context) error

// InitProvider initializes OpenTelemetry providers
func InitProvider(ctx context.Context, cfg Config) (ShutdownFunc, error) {
	if !cfg.Enabled {
		return func(ctx context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
		resource.WithHost(),
		resource.WithProcess(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	tracerProvider, err := initTracerProvider(ctx, cfg, res)
	if err != nil {
		return nil, fmt.Errorf("failed to init tracer provider: %w", err)
	}
	otel.SetTracerProvider(tracerProvider)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	loggerProvider, err := initLoggerProvider(ctx, cfg, res)
	if err != nil {
		return nil, fmt.Errorf("failed to init logger provider: %w", err)
	}
	global.SetLoggerProvider(loggerProvider)

	return func(ctx context.Context) error {
		var errs []error
		if err := tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
		if err := loggerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			return fmt.Errorf("shutdown errors: %v", errs)
		}
		return nil
	}, nil
}

func initTracerProvider(ctx context.Context, cfg Config, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(cfg.OTLPEndpoint+"/v1/traces"),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	), nil
}

func initLoggerProvider(ctx context.Context, cfg Config, res *resource.Resource) (*sdklog.LoggerProvider, error) {
	exporter, err := otlploghttp.New(ctx,
		otlploghttp.WithEndpointURL(cfg.OTLPEndpoint+"/v1/logs"),
		otlploghttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	return sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter,
			sdklog.WithExportInterval(5*time.Second),
			sdklog.WithExportMaxBatchSize(512),
		)),
		sdklog.WithResource(res),
	), nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
