package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"knowledge-sovereign/config"
	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/gen/proto/services/sovereign/v1/sovereignv1connect"
	"knowledge-sovereign/handler"
)

func newHTTPServer(addr string, h http.Handler, writeTimeout time.Duration) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

func logEventAuthStatus(enabled bool) {
	if enabled {
		slog.Info("event_auth_enabled")
	} else {
		slog.Warn("event_auth_disabled: EVENT_AUTH=disabled was set explicitly; event listener accepts unauthenticated RPCs")
	}
}

func buildMetricsMux(repo *sovereign_db.Repository, cfg *config.Config) *http.ServeMux {
	snapshotHandler := handler.NewSnapshotHandler(repo, cfg.SnapshotDir, cfg.BuildRef, cfg.SchemaVersion)
	retentionHandler := handler.NewRetentionHandler(repo, cfg.ArchiveDir)
	storageHandler := handler.NewStorageHandler(repo)
	// Projection rebuild: truncate an allowlisted read-model set and reset its
	// projector checkpoint in one transaction, so the in-process projectors
	// below re-fold the event log from the beginning. Codifies the procedure
	// that lived as raw SQL in docs/runbooks/knowledge-trail-reproject.md,
	// including the PM-2026-010 invariant that the two steps are inseparable.
	projectionRebuildHandler := handler.NewProjectionRebuildHandler(repo)

	metricsMux := http.NewServeMux()
	metricsMux.HandleFunc("/health", handler.HealthHandler)
	metricsMux.Handle("/health/deep", handler.NewDeepHealthHandler(repo))
	// Prometheus scrape endpoint. Default registry collectors include
	// process / Go runtime metrics out of the box; projection_health gauges
	// and projector counters register with the same default registry via promauto.
	metricsMux.Handle("/metrics", promhttp.Handler())
	snapshotHandler.RegisterRoutes(metricsMux)
	retentionHandler.RegisterRoutes(metricsMux)
	storageHandler.RegisterRoutes(metricsMux)
	projectionRebuildHandler.RegisterRoutes(metricsMux)
	return metricsMux
}

func buildRPCMux(sovereignHandler sovereignv1connect.KnowledgeSovereignServiceHandler, cfg *config.Config) *http.ServeMux {
	mainMux := http.NewServeMux()
	mainMux.HandleFunc("/health", handler.HealthHandler)

	path, rpcHandler := sovereignv1connect.NewKnowledgeSovereignServiceHandler(
		sovereignHandler,
		connect.WithInterceptors(handler.NewEventAuthInterceptor(cfg.EventToken, cfg.EventAuthEnabled)),
	)
	mainMux.Handle(path, rpcHandler)
	return mainMux
}

type serverGroup struct {
	metricsServer *http.Server
	mainServer    *http.Server
}

func startServers(cfg *config.Config, repo *sovereign_db.Repository, sovereignHandler sovereignv1connect.KnowledgeSovereignServiceHandler) *serverGroup {
	logAdminAuthStatus(cfg.AdminAuthEnabled)
	metricsMux := buildMetricsMux(repo, cfg)
	metricsServer := newHTTPServer(cfg.MetricsAddr, requireAdminToken(cfg.AdminToken, cfg.AdminAuthEnabled, metricsMux), 30*time.Second)

	go func() {
		slog.Info("metrics server starting", "addr", cfg.MetricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server failed", "error", err)
			os.Exit(1)
		}
	}()

	logEventAuthStatus(cfg.EventAuthEnabled)
	mainMux := buildRPCMux(sovereignHandler, cfg)
	// WriteTimeout is intentionally unset: WatchProjectorEvents is a
	// long-lived server-streaming RPC on this mux, and a finite write
	// deadline would sever it mid-stream. ReadHeaderTimeout/ReadTimeout/
	// IdleTimeout still guard against slowloris-style connection abuse.
	mainServer := newHTTPServer(cfg.ListenAddr, mainMux, 0)

	go func() {
		slog.Info("rpc server starting", "addr", cfg.ListenAddr)
		if err := mainServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("rpc server failed", "error", err)
			os.Exit(1)
		}
	}()

	return &serverGroup{
		metricsServer: metricsServer,
		mainServer:    mainServer,
	}
}

func (s *serverGroup) shutdown(ctx context.Context) {
	if err := s.metricsServer.Shutdown(ctx); err != nil {
		slog.Error("metrics server shutdown failed", "error", err)
	}
	if err := s.mainServer.Shutdown(ctx); err != nil {
		slog.Error("rpc server shutdown failed", "error", err)
	}
}
