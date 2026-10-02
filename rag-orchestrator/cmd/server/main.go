package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus"
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
	"rag-orchestrator/internal/pki"
)

func main() {
	if isHealthcheckCommand(os.Args) {
		port := extractHealthcheckPort(os.Args)
		os.Exit(runHealthcheck(port, nil))
	}

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
	serverErr := make(chan error, 3)

	pkiReg := prometheus.NewRegistry()
	pkiHandle, err := pki.StartWithRegisterer(ctx, log, "rag-orchestrator", pkiReg)
	if err != nil {
		log.Error("pki enrollment failed", "error_type", pki.LogSafeError(err))
		os.Exit(1)
	}
	defer pkiHandle.Stop()

	opsAddr, err := pki.LoadOpsListenAddr()
	if err != nil {
		log.Error("pki ops listen addr", "error", err)
		os.Exit(1)
	}
	opsServer := pki.NewOpsServer(opsAddr, pki.NewOpsHandler(pkiReg))
	go func() {
		log.Info("pki_ops_listener_enabled", "addr", opsAddr, "surfaces", "/health,/metrics")
		if err := opsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("pki ops listener failed", "error", err)
			serverErr <- err
		}
	}()

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

	if cfg.APIAuth.Enabled {
		log.Info("rag_api_auth_enabled", "surfaces", ":9010 echo REST")
	} else {
		log.Warn("rag_api_auth_disabled",
			"reason", "RAG_API_AUTH=disabled (explicit opt-out); :9010 listener is unauthenticated — exposure is limited only by network policy",
		)
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

	// 6.1 Start the corpus census sampler. Its gauge is the only signal that
	// shows a chunker/embedder version drift before it degrades retrieval.
	app.CoverageSampler.Start(ctx)
	defer app.CoverageSampler.Stop()

	// 7. Initialize Echo
	e := echo.New()
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	apiAuthMW := peermw.NewAPIAuthMiddleware(cfg.APIAuth.Token, cfg.APIAuth.Enabled, log)
	e.Use(apiAuthMW.EchoMiddleware())

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

	// 9.1 Prometheus /metrics on the Echo API mux. Exposes the
	// rag_orchestrator_knowledge_event_emitter_* counters (Knowledge Loop
	// Completion Phase 1 §1). PKI enrollment series live on the dedicated
	// ops listener (:9110, private registry) — they must not land here via
	// DefaultRegisterer.
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	// Dedicated Plaintext Health Check Server
	healthPort := os.Getenv("RAG_HEALTH_PORT")
	if healthPort == "" {
		healthPort = "9012"
	}
	healthServer := newHealthServer(":"+healthPort, dbPool, log)
	go func() {
		log.Info("Starting plaintext health server", "addr", healthServer.Addr)
		if err := healthServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("health server failed", "error", err)
			serverErr <- err
		}
	}()

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
	e.Server.Handler = e

	go func() {
		if connectTLS != nil {
			log.Info("Starting Echo server (TLS)", "addr", e.Server.Addr)
			e.Server.TLSConfig = connectTLS
			if err := e.Server.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("echo tls server failed", "error", err)
				serverErr <- err
			}
		} else {
			log.Info("Starting Echo server (plaintext)", "addr", e.Server.Addr)
			if err := e.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("echo plaintext server failed", "error", err)
				serverErr <- err
			}
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
	var connectServer *http.Server
	if cfg.PeerIdentity.Mode == config.PeerIdentityMTLS {
		connectServer, err = newProductionTLSServer(
			fmt.Sprintf(":%s", cfg.Server.ConnectPort),
			connectHandler,
			cfg.PeerIdentity.CertFile,
			cfg.PeerIdentity.KeyFile,
			cfg.PeerIdentity.CAFile,
		)
		if err != nil {
			log.Error("connect_tls_server_construct_failed", "error", err)
			os.Exit(1)
		}
	} else {
		connectServer = &http.Server{
			Addr:              fmt.Sprintf(":%s", cfg.Server.ConnectPort),
			Handler:           connectHandler,
			ReadHeaderTimeout: 10 * time.Second,
		}
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

	if err := opsServer.Shutdown(ctx); err != nil {
		log.Error("pki ops listener shutdown error", "error", err)
	}
	if err := healthServer.Shutdown(ctx); err != nil {
		log.Error("health server shutdown error", "error", err)
	}
	if err := connectServer.Shutdown(ctx); err != nil {
		log.Error("Connect-RPC server shutdown error", "error", err)
	}
	if err := e.Shutdown(ctx); err != nil {
		log.Error("echo server shutdown error", "error", err)
	}
}

// HealthPinger checks readiness of backing dependencies (e.g. database pool).
type HealthPinger interface {
	Ping(ctx context.Context) error
}

// newHealthMux builds the ServeMux for the dedicated public health check port.
// It exposes ONLY /healthz and /readyz, never business RPC routes.
func newHealthMux(pinger HealthPinger, log *slog.Logger) *http.ServeMux {
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if pinger != nil {
			if err := pinger.Ping(r.Context()); err != nil {
				if log != nil {
					log.Error("readyz db ping failed", "error", err)
				}
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"status":"db down"}`))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})
	return healthMux
}

// newHealthServer constructs the dedicated plaintext health HTTP server.
func newHealthServer(addr string, pinger HealthPinger, log *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           newHealthMux(pinger, log),
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// newProductionTLSServer constructs a production *http.Server configured with
// mTLS client authentication (RequireAndVerifyClientCert and TLS 1.3 minimum)
// loaded from the provided cert, key, and CA bundle paths.
func newProductionTLSServer(addr string, handler http.Handler, certFile, keyFile, caFile string) (*http.Server, error) {
	tlsCfg, err := tlsutil.LoadServerConfig(certFile, keyFile, caFile)
	if err != nil {
		return nil, err
	}
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 10 * time.Second,
	}, nil
}

// isHealthcheckCommand returns true if "healthcheck" is present in args.
func isHealthcheckCommand(args []string) bool {
	for i := 1; i < len(args); i++ {
		if args[i] == "healthcheck" {
			return true
		}
	}
	return false
}

// extractHealthcheckPort determines the healthcheck port from CLI args, environment, or default "9012".
func extractHealthcheckPort(args []string) string {
	for i := 1; i < len(args); i++ {
		if args[i] == "healthcheck" {
			for j := i + 1; j < len(args); j++ {
				arg := args[j]
				if (arg == "-port" || arg == "--port") && j+1 < len(args) {
					return args[j+1]
				}
				if strings.HasPrefix(arg, "-port=") {
					return strings.TrimPrefix(arg, "-port=")
				}
				if strings.HasPrefix(arg, "--port=") {
					return strings.TrimPrefix(arg, "--port=")
				}
				if !strings.HasPrefix(arg, "-") && arg != "" {
					return arg
				}
			}
			break
		}
	}
	if envPort := os.Getenv("RAG_HEALTH_PORT"); envPort != "" {
		return envPort
	}
	return "9012"
}

// validateHealthPort ensures the port is strictly decimal digits in range 1..65535,
// preventing userinfo or host manipulation before any URL is built.
func validateHealthPort(portStr string) (int, error) {
	if portStr == "" {
		return 0, errors.New("port cannot be empty")
	}
	for i := 0; i < len(portStr); i++ {
		if portStr[i] < '0' || portStr[i] > '9' {
			return 0, fmt.Errorf("invalid port character %q: strict decimal digits required", portStr[i])
		}
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("invalid port: %w", err)
	}
	if p < 1 || p > 65535 {
		return 0, fmt.Errorf("port out of range 1..65535: %d", p)
	}
	return p, nil
}

// runHealthcheck performs a bounded HTTP GET against the dedicated plaintext health endpoint.
// It checks loopback only with a 5s maximum timeout, requires HTTP 200, reads a bounded body (max 4KiB,
// failing closed if oversize), does not follow redirects, and ensures the body is closed.
// Returns 0 on success, non-zero on error.
func runHealthcheck(port string, client *http.Client) int {
	portNum, err := validateHealthPort(port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", err)
		return 1
	}

	var httpClient http.Client
	if client != nil {
		httpClient = *client // shallow clone to prevent mutating caller's pointer
	}
	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	if httpClient.Timeout <= 0 || httpClient.Timeout > 5*time.Second {
		httpClient.Timeout = 5 * time.Second
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/healthz", portNum)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil) // #nosec G704 -- fixed loopback host and validated numeric port; no user URL
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck request build failed: %v\n", err)
		return 1
	}
	resp, err := httpClient.Do(req) // #nosec G704 -- fixed loopback request; redirects disabled above
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()

	// Body bounded to max 4KiB (4096 bytes); oversize fails closed.
	const maxHealthBodyBytes int64 = 4096
	lr := io.LimitReader(resp.Body, maxHealthBodyBytes+1)
	n, _ := io.Copy(io.Discard, lr)
	if n > maxHealthBodyBytes {
		fmt.Fprintf(os.Stderr, "healthcheck failed: response body exceeded limit of %d bytes\n", maxHealthBodyBytes)
		return 1
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck failed: status %d\n", resp.StatusCode)
		return 1
	}
	return 0
}
