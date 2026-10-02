package otel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
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

// DefaultRaskIngestTokenFile is the default path to the Rask ingest token secret.
const DefaultRaskIngestTokenFile = "/run/secrets/rask_ingest_token"

// ResolveRaskIngestToken resolves the Bearer token for authenticating OTLP telemetry
// export to rask-log-aggregator. It loads from RASK_INGEST_TOKEN_FILE (defaulting to
// /run/secrets/rask_ingest_token), or RASK_INGEST_TOKEN if set directly.
//
// Fails fast if the file cannot be read or resolves to an empty token.
// Telemetry credentials MUST NEVER be logged.
func ResolveRaskIngestToken() (string, error) {
	if token := strings.TrimSpace(os.Getenv("RASK_INGEST_TOKEN")); token != "" {
		return token, nil
	}
	filePath := strings.TrimSpace(os.Getenv("RASK_INGEST_TOKEN_FILE"))
	if filePath == "" {
		filePath = DefaultRaskIngestTokenFile
	}
	content, err := os.ReadFile(filePath) // #nosec G304 -- trusted operator secret path
	if err != nil {
		return "", fmt.Errorf("read RASK_INGEST_TOKEN_FILE %s: %w", filePath, err)
	}
	token := strings.TrimSpace(string(content))
	if token == "" {
		return "", fmt.Errorf("RASK_INGEST_TOKEN_FILE %s resolved to an empty token", filePath)
	}
	return token, nil
}

// Config holds OpenTelemetry configuration
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	OTLPEndpoint   string
	Enabled        bool
	IngestToken    string // Bearer token for OTLP export to rask-log-aggregator
}

// ConfigFromEnv creates Config from environment variables
func ConfigFromEnv() Config {
	enabled := getEnv("OTEL_ENABLED", "true") == "true"
	return Config{
		ServiceName:    getEnv("OTEL_SERVICE_NAME", "pre-processor"),
		ServiceVersion: getEnv("SERVICE_VERSION", "0.0.0"),
		Environment:    getEnv("DEPLOYMENT_ENV", "development"),
		OTLPEndpoint:   getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318"),
		Enabled:        enabled,
	}
}

// ShutdownFunc is a function to shutdown providers
type ShutdownFunc func(context.Context) error

var tokenRegex = regexp.MustCompile(`^[A-Za-z0-9._~+/-]+=*$`)

// InitProvider initializes OpenTelemetry providers
func InitProvider(ctx context.Context, cfg Config) (ShutdownFunc, error) {
	if !cfg.Enabled {
		return func(ctx context.Context) error { return nil }, nil
	}

	token := cfg.IngestToken
	if token == "" {
		var err error
		token, err = ResolveRaskIngestToken()
		if err != nil {
			return nil, fmt.Errorf("telemetry enabled but failed to resolve ingest token: %w", err)
		}
	}
	cfg.IngestToken = token

	// Create resource
	if cfg.IngestToken == "" || !tokenRegex.MatchString(cfg.IngestToken) {
		return nil, fmt.Errorf("invalid ingest token format")
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

	// Initialize Trace Provider
	tracerProvider, err := initTracerProvider(ctx, cfg, res)
	if err != nil {
		return nil, fmt.Errorf("failed to init tracer provider: %w", err)
	}
	otel.SetTracerProvider(tracerProvider)

	// Set propagator for context propagation (W3C Trace Context)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Initialize Log Provider
	loggerProvider, err := initLoggerProvider(ctx, cfg, res)
	if err != nil {
		return nil, fmt.Errorf("failed to init logger provider: %w", err)
	}
	global.SetLoggerProvider(loggerProvider)

	// Return shutdown function
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

// buildHTTPTransport constructs an *http.Transport that honours the standard
// OTEL TLS environment variables with signal-specific precedence:
//
//	OTEL_EXPORTER_OTLP_<sigEnvPrefix>_CERTIFICATE      > OTEL_EXPORTER_OTLP_CERTIFICATE
//	OTEL_EXPORTER_OTLP_<sigEnvPrefix>_CLIENT_CERTIFICATE > OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE
//	OTEL_EXPORTER_OTLP_<sigEnvPrefix>_CLIENT_KEY        > OTEL_EXPORTER_OTLP_CLIENT_KEY
//
// sigEnvPrefix must be "TRACES" or "LOGS".
// When no TLS env vars are set the returned transport is nil (default system CAs).
// minTLS is 1.2; ServerName verification is always on.
func buildHTTPTransport(sigEnvPrefix string) (*http.Transport, error) {
	envCA := envSignalOrCommon(sigEnvPrefix, "CERTIFICATE")
	envClientCert := envSignalOrCommon(sigEnvPrefix, "CLIENT_CERTIFICATE")
	envClientKey := envSignalOrCommon(sigEnvPrefix, "CLIENT_KEY")

	if envCA == "" && envClientCert == "" && envClientKey == "" {
		return nil, nil //nolint:nilnil // no TLS env — caller uses default transport
	}

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}

	if envCA != "" {
		pem, err := os.ReadFile(envCA) // #nosec G304 -- operator-controlled CA path
		if err != nil {
			return nil, fmt.Errorf("OTLP CA cert %s: %w", envCA, err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("OTLP CA cert %s: no valid PEM certificate found", envCA)
		}
		tlsCfg.RootCAs = pool
	}

	if envClientCert != "" || envClientKey != "" {
		if envClientCert == "" || envClientKey == "" {
			return nil, fmt.Errorf("OTLP mTLS requires both CLIENT_CERTIFICATE and CLIENT_KEY (signal %s)", sigEnvPrefix)
		}
		cert, err := tls.LoadX509KeyPair(envClientCert, envClientKey)
		if err != nil {
			return nil, fmt.Errorf("OTLP mTLS key pair (signal %s): %w", sigEnvPrefix, err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return &http.Transport{TLSClientConfig: tlsCfg}, nil
}

// envSignalOrCommon returns OTEL_EXPORTER_OTLP_<sigEnvPrefix>_<suffix> if set,
// otherwise OTEL_EXPORTER_OTLP_<suffix>.
func envSignalOrCommon(sigEnvPrefix, suffix string) string {
	if v := os.Getenv("OTEL_EXPORTER_OTLP_" + sigEnvPrefix + "_" + suffix); v != "" {
		return v
	}
	return os.Getenv("OTEL_EXPORTER_OTLP_" + suffix)
}

func initTracerProvider(ctx context.Context, cfg Config, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	u, err := url.Parse(cfg.OTLPEndpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid OTLP endpoint: must be http or https with a hostname")
	}

	transport, err := buildHTTPTransport("TRACES")
	if err != nil {
		return nil, fmt.Errorf("OTLP trace TLS: %w", err)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if transport != nil {
		client.Transport = transport
	}

	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpointURL(cfg.OTLPEndpoint + "/v1/traces"),
		otlptracehttp.WithHTTPClient(client),
	}
	if u.Scheme == "http" {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	if cfg.IngestToken != "" {
		opts = append(opts, otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Bearer " + cfg.IngestToken,
		}))
	}

	exporter, err := otlptracehttp.New(ctx, opts...)
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
	u, err := url.Parse(cfg.OTLPEndpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid OTLP endpoint: must be http or https with a hostname")
	}

	transport, err := buildHTTPTransport("LOGS")
	if err != nil {
		return nil, fmt.Errorf("OTLP log TLS: %w", err)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if transport != nil {
		client.Transport = transport
	}

	opts := []otlploghttp.Option{
		otlploghttp.WithEndpointURL(cfg.OTLPEndpoint + "/v1/logs"),
		otlploghttp.WithHTTPClient(client),
	}
	if u.Scheme == "http" {
		opts = append(opts, otlploghttp.WithInsecure())
	}
	if cfg.IngestToken != "" {
		opts = append(opts, otlploghttp.WithHeaders(map[string]string{
			"Authorization": "Bearer " + cfg.IngestToken,
		}))
	}

	exporter, err := otlploghttp.New(ctx, opts...)
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
