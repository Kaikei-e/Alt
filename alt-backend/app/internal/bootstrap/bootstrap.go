// Package bootstrap holds the startup sequence shared by every alt-backend
// binary (cmd/backend, cmd/harvester, cmd/datahub): config load, OpenTelemetry
// init, logger swap, and the database pool — in that exact order.
//
// The order matters and is the reason this lives in one place. The logger
// package's init() has already called slog.SetDefault, and InitLoggerWithOTel
// swaps that handler in place; the DI containers capture slog.Default() when
// they emit their Rule 8 wiring logs. Building a container before Boot has run
// therefore sends every wiring log to the bare stderr handler instead of OTel,
// which is why the containers take a *Runtime-derived pool and config rather
// than constructing their own.
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"alt/config"
	"alt/shared/driver/alt_db"
	"alt/utils/logger"
	altotel "alt/utils/otel"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Options describes what a binary needs from the shared startup sequence.
type Options struct {
	// ServiceName is the binary's OpenTelemetry service.name. It must match
	// OTEL_SERVICE_NAME when that variable is set.
	ServiceName string
	// RequireDB opens the alt-db pool. All three binaries set this during the
	// transition period, when each still talks to alt-db directly.
	RequireDB bool
}

// Runtime is what a booted process holds: validated config, the OTel-aware
// logger, the database pool, and the Prometheus handler for /metrics.
type Runtime struct {
	Cfg            *config.Config
	Log            *slog.Logger
	Pool           *pgxpool.Pool
	MetricsHandler http.Handler

	shutdownHooks []func(context.Context) error
}

// Shutdown releases everything Boot acquired, in reverse acquisition order.
func (r *Runtime) Shutdown(ctx context.Context) {
	for i := len(r.shutdownHooks) - 1; i >= 0; i-- {
		if err := r.shutdownHooks[i](ctx); err != nil {
			r.Log.ErrorContext(ctx, "shutdown hook failed", "error", err)
		}
	}
}

// MustBoot runs the shared startup sequence or exits non-zero. Every failure
// here is missing or contradictory required config, which CLAUDE.md rule 9
// makes a startup failure rather than a warn-and-limp.
func MustBoot(ctx context.Context, opts Options) *Runtime {
	bootLog := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if strings.TrimSpace(opts.ServiceName) == "" {
		bootLog.Error("bootstrap.Options.ServiceName is required")
		os.Exit(1)
	}

	cfg, err := config.NewConfig()
	if err != nil {
		bootLog.Error("failed to load configuration", "error", err, "service", opts.ServiceName)
		os.Exit(1)
	}

	otelCfg := altotel.ConfigFromEnv()
	serviceName, err := resolveServiceName(os.Getenv("OTEL_SERVICE_NAME"), opts.ServiceName)
	if err != nil {
		bootLog.Error("otel_service_name_mismatch", "error", err,
			"binary", opts.ServiceName, "otel_service_name", os.Getenv("OTEL_SERVICE_NAME"))
		os.Exit(1)
	}
	otelCfg.ServiceName = serviceName

	otelResult, err := altotel.InitProviderWithMetrics(ctx, otelCfg)
	if err != nil {
		bootLog.Error("failed to initialize OpenTelemetry", "error", err)
		// Continue without OTel — non-fatal, matching the single-binary behaviour.
		otelCfg.Enabled = false
		otelResult = &altotel.InitResult{
			Shutdown: func(context.Context) error { return nil },
		}
	}

	log := logger.InitLoggerWithOTel(otelCfg.Enabled)

	rt := &Runtime{
		Cfg:            cfg,
		Log:            log,
		MetricsHandler: otelResult.MetricsHandler,
	}
	rt.shutdownHooks = append(rt.shutdownHooks, func(ctx context.Context) error {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := otelResult.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown opentelemetry: %w", err)
		}
		return nil
	})

	log.InfoContext(ctx, "starting service",
		"service", serviceName,
		"otel_enabled", otelCfg.Enabled,
		"app_env", cfg.AppEnv,
	)
	log.InfoContext(ctx, "Go runtime settings",
		"GOMEMLIMIT", os.Getenv("GOMEMLIMIT"),
		"GOGC", os.Getenv("GOGC"),
		"GOMAXPROCS", runtime.GOMAXPROCS(0),
		"go_version", runtime.Version(),
		"pid", os.Getpid(),
	)

	if opts.RequireDB {
		pool, err := alt_db.InitDBConnectionPool(ctx)
		if err != nil {
			log.ErrorContext(ctx, "failed to connect to database", "error", err, "service", serviceName)
			panic(fmt.Errorf("init db connection pool: %w", err))
		}
		rt.Pool = pool
		rt.shutdownHooks = append(rt.shutdownHooks, func(context.Context) error {
			pool.Close()
			return nil
		})
	}

	return rt
}

// resolveServiceName reconciles OTEL_SERVICE_NAME with the name the binary
// claims. An unset (or whitespace-only) variable adopts the binary's name; a
// value that names a *different* binary is a leftover from the pre-split
// compose definition and fails startup, because the three processes emit the
// same instrument names and would otherwise silently merge into one series.
func resolveServiceName(envValue, binaryName string) (string, error) {
	env := strings.TrimSpace(envValue)
	if env == "" || env == binaryName {
		return binaryName, nil
	}
	return "", fmt.Errorf("OTEL_SERVICE_NAME=%q does not match this binary (%q): "+
		"metrics from all three binaries would collapse into one service", env, binaryName)
}

// LogMemStats records the final heap snapshot before shutdown, preserved from
// the single-binary main.go so the split does not lose the diagnostic.
func LogMemStats(ctx context.Context, log *slog.Logger) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	log.InfoContext(ctx, "final memory stats before shutdown",
		"alloc_mib", ms.Alloc/1024/1024,
		"sys_mib", ms.Sys/1024/1024,
		"heap_objects", ms.HeapObjects,
		"num_gc", ms.NumGC,
		"gc_cpu_fraction", ms.GCCPUFraction,
	)
}
