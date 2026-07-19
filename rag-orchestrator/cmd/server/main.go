package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	connectserver "rag-orchestrator/internal/adapter/connect"
	rag_http "rag-orchestrator/internal/adapter/rag_http"
	"rag-orchestrator/internal/adapter/rag_http/openapi"
	"rag-orchestrator/internal/di"
	"rag-orchestrator/internal/infra"
	"rag-orchestrator/internal/infra/config"
	"rag-orchestrator/internal/infra/logger"
	"rag-orchestrator/internal/infra/otel"
	"rag-orchestrator/internal/infra/tlsutil"
	peermw "rag-orchestrator/internal/middleware"
)

func main() {
	ctx := context.Background()

	// 1. Load Config
	cfg := config.Load()

	// 2. Initialize OpenTelemetry
	otelCfg := otel.ConfigFromEnv()
	shutdown, err := otel.InitProvider(ctx, otelCfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to initialize OTel provider", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := shutdown(ctx); err != nil {
			slog.ErrorContext(ctx, "failed to shutdown OTel provider", "error", err)
		}
	}()

	// 3. Initialize Logger with OTel support (also installs slog.SetDefault)
	log := logger.NewWithOTel(otelCfg.Enabled)

	// PEER_IDENTITY_MODE is required (config.Load fails hard when unset).
	// "mtls" terminates TLS on the Connect-RPC listener and wires
	// PeerIdentityMiddleware so X-Alt-User-Id is only trusted from verified
	// peers; "disabled" is an explicit opt-out that keeps the plaintext h2c
	// listener. Either way the wiring state is logged loudly at startup
	// (CLAUDE.md rules 8/9 / .claude/rules/di-wiring.md).
	var (
		connectTLS *tls.Config
		peerMW     *peermw.PeerIdentityMiddleware
	)
	switch cfg.PeerIdentity.Mode {
	case config.PeerIdentityMTLS:
		connectTLS, err = tlsutil.LoadServerConfig(cfg.PeerIdentity.CertFile, cfg.PeerIdentity.KeyFile, cfg.PeerIdentity.CAFile)
		if err != nil {
			log.Error("peer_identity_tls_config_failed", "error", err)
			os.Exit(1)
		}
		peerMW = peermw.NewPeerIdentityMiddleware(cfg.PeerIdentity.AllowedPeers, log)
		log.Info("peer_identity_enabled",
			"mode", "mtls",
			"allowed_peers", cfg.PeerIdentity.AllowedPeers,
		)
	case config.PeerIdentityDisabled:
		log.Warn("peer_identity_disabled",
			"reason", "PEER_IDENTITY_MODE=disabled (explicit opt-out); Connect-RPC listener stays plaintext h2c and X-Alt-User-Id is unverified — exposure is limited only by network policy",
		)
	default:
		log.Error("peer_identity_mode_unhandled", "mode", string(cfg.PeerIdentity.Mode))
		os.Exit(1)
	}

	// 4. Initialize DB
	dbPool, err := infra.NewPostgresDB(ctx, cfg.DB.DSN(), infra.PoolConfig{
		MaxConns: cfg.DB.MaxConns,
		MinConns: cfg.DB.MinConns,
	})
	if err != nil {
		log.Error("failed to connect to db", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	// 5. Wire all dependencies
	app := di.NewApplicationComponents(cfg, dbPool, log)

	// 6. Start Worker
	app.Worker.Start()
	defer func() {
		log.Info("Stopping worker...")
		app.Worker.Stop()
	}()

	// 7. Initialize Echo
	e := echo.New()
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	// 8. Initialize Handlers
	handler := rag_http.NewHandler(
		app.RetrieveUsecase,
		app.AnswerUsecase,
		app.IndexUsecase,
		app.JobRepo,
		app.MorningLetterUsecase,
		log,
		rag_http.WithEmbedderOverride(app.EmbedderFactory, app.IndexUsecaseFactory, app.EmbeddingModel, app.EmbedderTimeout, cfg.Embedder.AllowedOverrideOrigins),
	)
	openapi.RegisterHandlers(e, handler)
	e.POST("/internal/rag/backfill", handler.Backfill)
	e.POST("/v1/rag/morning-letter", handler.MorningLetter)

	// 9. Health Checks
	e.GET("/healthz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	e.GET("/readyz", func(c echo.Context) error {
		if err := dbPool.Ping(c.Request().Context()); err != nil {
			log.Error("readyz db ping failed", "error", err)
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "db down"})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
	})

	// 9.1 Prometheus /metrics. Exposes the rag_orchestrator_knowledge_event_emitter_*
	// counters (Knowledge Loop Completion Phase 1 §1) and any future
	// promauto-registered process metrics. The default registry is fine — no
	// need to namespace per-instance because the Prometheus scrape job
	// already labels rows with service="rag-orchestrator".
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	// 10. Start Echo Server
	// ReadHeaderTimeout/ReadTimeout/IdleTimeout/MaxHeaderBytes are set
	// explicitly to avoid the bare http.Server defaults (unlimited = open
	// to Slowloris). WriteTimeout stays 0 (unlimited) because the SSE
	// streaming endpoints (handler.go writeSSE) hold the response open.
	e.Server.Addr = fmt.Sprintf(":%s", cfg.Server.Port)
	e.Server.ReadHeaderTimeout = 10 * time.Second
	e.Server.ReadTimeout = 30 * time.Second
	e.Server.IdleTimeout = 120 * time.Second
	e.Server.MaxHeaderBytes = 1 << 20 // 1 MiB
	serverErr := make(chan error, 2)
	go func() {
		log.Info("Starting Echo server", "addr", e.Server.Addr)
		if err := e.StartServer(e.Server); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "error", err)
			serverErr <- err
		}
	}()

	// 11. Start Connect-RPC Server. In mtls mode the listener terminates TLS
	// (RequireAndVerifyClientCert) and PeerIdentityMiddleware gates every RPC;
	// in disabled mode it keeps the historical plaintext h2c behaviour.
	var connectHandler http.Handler
	if cfg.PeerIdentity.Mode == config.PeerIdentityMTLS {
		connectHandler = connectserver.CreateMTLSConnectServer(peerMW, app.ArticleClient, app.AnswerUsecase, app.RetrieveUsecase, app.ConversationUsecase, app.EventEmitter, app.LetterFetcher, log)
	} else {
		connectHandler = connectserver.CreateConnectServer(app.ArticleClient, app.AnswerUsecase, app.RetrieveUsecase, app.ConversationUsecase, app.EventEmitter, app.LetterFetcher, log)
	}
	connectServer := &http.Server{
		Addr:              fmt.Sprintf(":%s", cfg.Server.ConnectPort),
		Handler:           connectHandler,
		TLSConfig:         connectTLS,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Info("Starting Connect-RPC server", "addr", connectServer.Addr, "peer_identity_mode", string(cfg.PeerIdentity.Mode))
		var err error
		if connectTLS != nil {
			// Cert/key come from TLSConfig.GetCertificate (hot-reloaded), so
			// the file arguments stay empty.
			err = connectServer.ListenAndServeTLS("", "")
		} else {
			err = connectServer.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("Connect-RPC server error", "error", err)
			serverErr <- err
		}
	}()

	// 12. Graceful Shutdown — signal path and server-failure path share the same
	// teardown so defer (worker stop / DB close / OTel shutdown) always runs.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-quit:
		log.Info("shutdown signal received", "signal", sig.String())
	case err := <-serverErr:
		log.Error("server failed, shutting down", "error", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := connectServer.Shutdown(ctx); err != nil {
		log.Error("Connect-RPC server shutdown error", "error", err)
	}
	if err := e.Shutdown(ctx); err != nil {
		log.Error("echo server shutdown error", "error", err)
	}
}
