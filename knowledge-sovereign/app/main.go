package main

import (
	"context"
	"crypto/subtle"
	"knowledge-sovereign/config"
	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/gen/proto/services/sovereign/v1/sovereignv1connect"
	"knowledge-sovereign/handler"
	"knowledge-sovereign/usecase/knowledge_home_projector"
	"knowledge-sovereign/usecase/knowledge_trail_projector"
	"knowledge-sovereign/usecase/projection_health"
	"knowledge-sovereign/usecase/trail_planner"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Database connection
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("database ping failed", "error", err)
		os.Exit(1)
	}
	slog.Info("database connected")

	// Initialize layers
	repo := sovereign_db.NewRepository(pool)
	sovereignHandler := handler.NewSovereignHandler(repo, handler.WithDatabaseURL(cfg.DatabaseURL))

	snapshotHandler := handler.NewSnapshotHandler(repo, cfg.SnapshotDir, cfg.BuildRef, cfg.SchemaVersion)
	retentionHandler := handler.NewRetentionHandler(repo, cfg.ArchiveDir)
	storageHandler := handler.NewStorageHandler(repo)

	// Metrics / health server
	metricsMux := http.NewServeMux()
	metricsMux.HandleFunc("/health", handler.HealthHandler)
	// Prometheus scrape endpoint. Default registry collectors include
	// process / Go runtime metrics out of the box; projection_health gauges
	// and projector counters register with the same default registry via promauto.
	metricsMux.Handle("/metrics", promhttp.Handler())
	snapshotHandler.RegisterRoutes(metricsMux)
	retentionHandler.RegisterRoutes(metricsMux)
	storageHandler.RegisterRoutes(metricsMux)

	if cfg.AdminToken == "" {
		slog.Warn("admin_auth_disabled: /admin/* endpoints on the metrics port accept unauthenticated requests; set ADMIN_TOKEN to require a Bearer token")
	} else {
		slog.Info("admin_auth_enabled")
	}

	metricsServer := &http.Server{
		Addr:              cfg.MetricsAddr,
		Handler:           requireAdminToken(cfg.AdminToken, metricsMux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		slog.Info("metrics server starting", "addr", cfg.MetricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Main RPC server with Connect-RPC handlers
	mainMux := http.NewServeMux()
	mainMux.HandleFunc("/health", handler.HealthHandler)

	path, rpcHandler := sovereignv1connect.NewKnowledgeSovereignServiceHandler(sovereignHandler)
	mainMux.Handle(path, rpcHandler)

	// WriteTimeout is intentionally unset: WatchProjectorEvents is a
	// long-lived server-streaming RPC on this mux, and a finite write
	// deadline would sever it mid-stream. ReadHeaderTimeout/ReadTimeout/
	// IdleTimeout still guard against slowloris-style connection abuse.
	mainServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mainMux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		slog.Info("rpc server starting", "addr", cfg.ListenAddr)
		if err := mainServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("rpc server failed", "error", err)
			os.Exit(1)
		}
	}()

	var wg sync.WaitGroup

	// Knowledge Trail spine projector. Folds the append-only event log into
	// knowledge_trail_footprints in-process. Reproject-safe and idempotent, so a
	// short tick over a quiet log is cheap.
	trailProjector := knowledge_trail_projector.NewProjector(repo, slog.Default(),
		knowledge_trail_projector.Config{
			BatchSize:         cfg.TrailProjectorBatchSize,
			MaxBatchesPerTick: cfg.TrailProjectorMaxBatches,
		})
	trailTick := time.NewTicker(cfg.ProjectorTickInterval)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer trailTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-trailTick.C:
				if err := trailProjector.RunBatch(ctx); err != nil {
					slog.Error("knowledge_trail_projector batch failed", "error", err)
				}
			}
		}
	}()

	// Knowledge Home projector. Folds the same append-only event log into the
	// Knowledge Home read models (knowledge_home_items, today_digest_view,
	// recall_candidate_view) in-process. Reproject-safe and idempotent, same
	// shape as knowledge_trail_projector above; shares its tick interval since
	// both drain the same event log on the same cadence. Rule 8: surface the
	// wiring state loudly at startup so a missing projector is not
	// indistinguishable from an intentionally-disabled one (PM-2026-045 /
	// ADR-000928).
	homeProjector := knowledge_home_projector.NewProjector(repo, slog.Default(),
		knowledge_home_projector.Config{
			BatchSize:         cfg.HomeProjectorBatchSize,
			MaxBatchesPerTick: cfg.HomeProjectorMaxBatches,
		})
	slog.Info("home.projector.wiring", "enabled", true, "repository_wired", repo != nil)
	homeTick := time.NewTicker(cfg.ProjectorTickInterval)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer homeTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-homeTick.C:
				if err := homeProjector.RunBatch(ctx); err != nil {
					slog.Error("knowledge_home_projector batch failed", "error", err)
				}
			}
		}
	}()

	// Knowledge Trail branch producer (trail_planner). Rule 8: surface the wiring
	// state loudly at startup so a missing producer is visible immediately, not
	// as a silent absence of branches weeks later (PM-2026-045 / ADR-000928).
	// NewPlanner always returns non-nil; the real wiring signal is whether the
	// repository dependency was supplied.
	branchPlanner := trail_planner.NewPlanner(repo, slog.Default(), trail_planner.Config{
		MaxBranchesPerUser: cfg.TrailMaxBranchesPerUser,
		Clock:              time.Now,
	})
	slog.Info("trail.branch_producer.wiring", "enabled", true, "repository_wired", repo != nil)
	branchTick := time.NewTicker(cfg.BranchPlannerTickInterval)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer branchTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-branchTick.C:
				if err := branchPlanner.RunBatch(ctx); err != nil {
					slog.Error("trail_planner batch failed", "error", err)
				}
			}
		}
	}()

	// Producer-liveness gauges sampled on a slow tick.
	healthExporter := projection_health.New(repo, slog.Default())
	healthTick := time.NewTicker(cfg.ProjectionHealthTickInterval)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer healthTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-healthTick.C:
				if err := healthExporter.RunOnce(ctx); err != nil {
					slog.Error("projection_health exporter failed", "error", err)
				}
			}
		}
	}()

	slog.Info("knowledge-sovereign started",
		"listen", cfg.ListenAddr,
		"metrics", cfg.MetricsAddr)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("metrics server shutdown failed", "error", err)
	}
	if err := mainServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("rpc server shutdown failed", "error", err)
	}

	cancel()
	wg.Wait()
	slog.Info("shutdown complete")
}

// requireAdminToken wraps next so that /admin/* requests must carry
// "Authorization: Bearer <token>" matching the configured admin token.
// If token is empty, admin auth is disabled and every request passes
// through unchanged (see the admin_auth_disabled startup log).
func requireAdminToken(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/admin/") {
			next.ServeHTTP(w, r)
			return
		}
		const prefix = "Bearer "
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, prefix) ||
			subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(auth, prefix)), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
