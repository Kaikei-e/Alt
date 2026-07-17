package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"pre-processor-sidecar/config"
	"pre-processor-sidecar/driver"
	"pre-processor-sidecar/handler"
	"pre-processor-sidecar/repository"
	"pre-processor-sidecar/security"
	"pre-processor-sidecar/service"
	"pre-processor-sidecar/service/scheduler"
	"pre-processor-sidecar/utils"

	// Import new scheduler package
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

// getNamespace gets the current Kubernetes namespace
func getNamespace() string {
	// Try to read from mounted service account token
	if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		return strings.TrimSpace(string(data))
	}

	// Fallback to environment variable
	if ns := os.Getenv("KUBERNETES_NAMESPACE"); ns != "" {
		return strings.TrimSpace(ns)
	}

	// Default namespace
	return "alt-processing"
}

// SimpleAdminAPIMetricsCollector はシンプルなメトリクス収集実装
type SimpleAdminAPIMetricsCollector struct {
	logger *slog.Logger
}

func (s *SimpleAdminAPIMetricsCollector) IncrementAdminAPIRequest(method, endpoint, status string) {
	s.logger.Debug("Admin API request", "method", method, "endpoint", endpoint, "status", status)
}

func (s *SimpleAdminAPIMetricsCollector) RecordAdminAPIRequestDuration(method, endpoint string, duration time.Duration) {
	s.logger.Debug("Admin API request duration", "method", method, "endpoint", endpoint, "duration_ms", duration.Milliseconds())
}

func (s *SimpleAdminAPIMetricsCollector) IncrementAdminAPIRateLimitHit() {
	s.logger.Warn("Admin API rate limit hit")
}

func (s *SimpleAdminAPIMetricsCollector) IncrementAdminAPIAuthenticationError(errorType string) {
	s.logger.Error("Admin API authentication error", "error_type", errorType)
}

func main() {
	// Parse command line flags
	healthCheck := flag.Bool("health-check", false, "Perform health check and exit")
	oauth2Init := flag.Bool("oauth2-init", false, "Initialize OAuth2 tokens and exit")
	scheduleMode := flag.Bool("schedule-mode", false, "Enable dual schedule processing mode")
	flag.Parse()

	// Setup structured logging
	logLevel := os.Getenv("LOG_LEVEL")
	var level slog.Level
	switch logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	if *healthCheck {
		performHealthCheck()
		return
	}

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	if *oauth2Init {
		performOAuth2Initialization(cfg, logger)
		return
	}

	// SIGTERM/SIGINT cancel ctx so <-ctx.Done() actually returns on shutdown and
	// the deferred cleanups in runScheduleMode (admin server shutdown, scheduler
	// stop, pool close, token rotation stop) run instead of being killed outright.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("Pre-processor-sidecar Scheduler starting with Simple Token System",
		"service", cfg.ServiceName,
		"subscription_sync_interval", "12h",
		"article_fetch_interval", "30m",
		"api_daily_limit", cfg.RateLimit.DailyLimit,
		"max_daily_rotations", os.Getenv("MAX_DAILY_ROTATIONS"),
		"batch_size", os.Getenv("BATCH_SIZE"))

	// Simple Token System初期化
	// Debug: Log OAuth2 base URL configuration
	logger.Info("OAuth2 configuration loaded",
		"oauth2_base_url", cfg.OAuth2.BaseURL,
		"inoreader_base_url", cfg.Inoreader.BaseURL)

	// Initialize token repository based on configuration
	var tokenRepo repository.OAuth2TokenRepository

	// Use RemoteTokenRepository - Centralized Token Management
	authTokenManagerURL := os.Getenv("AUTH_TOKEN_MANAGER_URL")
	if authTokenManagerURL == "" {
		authTokenManagerURL = "http://auth-token-manager:9201"
	}
	logger.Info("Using remote token repository", "url", authTokenManagerURL)
	tokenRepo = repository.NewRemoteTokenRepository(authTokenManagerURL, cfg.InternalAuthToken, logger)

	// Initialize RemoteTokenService
	remoteRepo, ok := tokenRepo.(*repository.RemoteTokenRepository)
	if !ok {
		logger.Error("Token repository is not RemoteTokenRepository, but it is required for RemoteTokenService")
		os.Exit(1)
	}
	remoteTokenService := service.NewRemoteTokenService(remoteRepo, logger)

	// Admin API用のトークンマネージャーアダプター作成 (Remote implementation)
	tokenManagerAdapter := &RemoteAdminTokenManager{
		service: remoteTokenService,
	}

	// Run in continuous scheduling mode with new token system
	if *scheduleMode {
		logger.Info("Starting in schedule mode as requested by flag")
	}
	if err := runScheduleMode(ctx, cfg, logger, remoteTokenService, tokenManagerAdapter, remoteRepo); err != nil {
		logger.Error("Scheduler failed", "error", err)
		os.Exit(1)
	}
}

// RemoteAdminTokenManager implements handler.TokenManager for Admin API using RemoteTokenService
type RemoteAdminTokenManager struct {
	service *service.RemoteTokenService
}

func (m *RemoteAdminTokenManager) UpdateRefreshToken(ctx context.Context, refreshToken string, clientID, clientSecret string) error {
	return fmt.Errorf("manual refresh token update not supported in remote mode - use auth-token-manager directly")
}

func (m *RemoteAdminTokenManager) GetTokenStatus() service.TokenStatus {
	// Basic status for remote service
	return service.TokenStatus{
		IsAutoRefreshing: true, // Managed remotely
		NeedsRefresh:     false,
	}
}

func (m *RemoteAdminTokenManager) GetValidToken(ctx context.Context) (*service.TokenInfo, error) {
	token, err := m.service.GetValidToken(ctx)
	if err != nil {
		return nil, err
	}
	return &service.TokenInfo{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.ExpiresAt,
		TokenType:    token.TokenType,
	}, nil
}

func performHealthCheck() {
	// Comprehensive health check for scheduler
	performHealthCheckWithOutput()
}

// healthRealClock is the wall-clock implementation of handler.HealthClock used in production.
type healthRealClock struct{}

func (healthRealClock) Now() time.Time { return time.Now() }

// ingestionHealthAdapter joins InoreaderService (fetch + circuit breaker) with the token
// provider so handler.HealthHandler can read all three signals from a single value.
type ingestionHealthAdapter struct {
	inoreaderSvc *service.InoreaderService
	tokenSvc     *service.RemoteTokenService
}

func newIngestionHealthAdapter(inoreaderSvc *service.InoreaderService, tokenProvider service.TokenProvider) *ingestionHealthAdapter {
	a := &ingestionHealthAdapter{inoreaderSvc: inoreaderSvc}
	if rts, ok := tokenProvider.(*service.RemoteTokenService); ok {
		a.tokenSvc = rts
	}
	return a
}

func (a *ingestionHealthAdapter) LastSuccessfulFetch() time.Time {
	return a.inoreaderSvc.LastSuccessfulFetch()
}

func (a *ingestionHealthAdapter) CircuitBreakerState() string {
	return a.inoreaderSvc.CircuitBreakerState()
}

func (a *ingestionHealthAdapter) TokenAvailable() bool {
	if a.tokenSvc == nil {
		return true
	}
	return a.tokenSvc.TokenAvailable()
}

func performOAuth2Initialization(cfg *config.Config, logger *slog.Logger) {
	logger.Info("OAuth2 initialization starting", "service", "oauth2-init")

	// Wait for Linkerd proxy initialization
	logger.Info("Waiting for Linkerd proxy initialization...", "wait", cfg.ProxyInitWait)
	time.Sleep(cfg.ProxyInitWait)

	dbConnectionString := cfg.Database.PostgresURL()

	// Create database connection
	logger.Info("Attempting database connection",
		"host", cfg.Database.Host,
		"port", cfg.Database.Port,
		"user", cfg.Database.User,
		"dbname", cfg.Database.Name,
		"sslmode", cfg.Database.SSLMode)

	pool, err := pgxpool.New(context.Background(), dbConnectionString)
	if err != nil {
		logger.Error("Failed to create database connection", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Test database connection with retry logic
	maxRetries := 3
	for i := 1; i <= maxRetries; i++ {
		if err := pool.Ping(context.Background()); err != nil {
			logger.Warn("Database ping failed, retrying...",
				"attempt", i,
				"error", err)
			if i == maxRetries {
				logger.Error("OAuth2 initialization failed", "error", fmt.Errorf("failed to ping database after %d attempts: %w", maxRetries, err))
				os.Exit(1)
			}
			time.Sleep(time.Duration(i*5) * time.Second)
			continue
		}
		break
	}

	logger.Info("Database connection established successfully")

	// OAuth2 initialization completed
}

// runScheduleMode は新しい統合トークンシステムでスケジュールモードを実行
func runScheduleMode(ctx context.Context, cfg *config.Config, logger *slog.Logger, tokenProvider service.TokenProvider, tokenManagerAdapter handler.TokenManager, tokenRepo repository.OAuth2TokenRepository) error {
	logger.Info("Initializing dual schedule processing system")

	// Wait for Linkerd proxy initialization
	logger.Info("Waiting for Linkerd proxy initialization...", "wait", cfg.ProxyInitWait)
	time.Sleep(cfg.ProxyInitWait)

	dbConnectionString := cfg.Database.PostgresURL()

	logger.Info("Attempting database connection",
		"host", cfg.Database.Host,
		"port", cfg.Database.Port,
		"user", cfg.Database.User,
		"dbname", cfg.Database.Name,
		"sslmode", cfg.Database.SSLMode)

	poolCfg, err := pgxpool.ParseConfig(dbConnectionString)
	if err != nil {
		return fmt.Errorf("failed to parse database connection string: %w", err)
	}
	// Configure connection pool to prevent exhaustion
	poolCfg.MaxConns = 25
	poolCfg.MinConns = 5
	poolCfg.MaxConnLifetime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}
	defer pool.Close()

	// Test database connection with retry
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		if err := pool.Ping(ctx); err != nil {
			logger.Warn("Database ping failed, retrying...", "attempt", i+1, "error", err)
			if i == maxRetries-1 {
				return fmt.Errorf("failed to ping database after %d attempts: %w", maxRetries, err)
			}
			time.Sleep(time.Duration(i+1) * 5 * time.Second)
			continue
		}
		break
	}
	logger.Info("Database connection established", "user", cfg.Database.User)

	logger.Info("HTTP client configured", "proxy", cfg.Proxy.HTTPSProxy)

	// Initialize repositories
	articleRepo := repository.NewPostgreSQLArticleRepository(pool, logger)
	syncStateRepo := repository.NewPostgreSQLSyncStateRepository(pool, logger)
	subscriptionRepo := repository.NewPostgreSQLSubscriptionRepository(pool, logger)

	// OAuth2クライアントの作成（Enhanced Token Serviceと同じ設定）
	clientID := cfg.OAuth2.ClientID
	clientSecret := cfg.OAuth2.ClientSecret
	oauth2Client := driver.NewOAuth2Client(clientID, clientSecret, cfg.OAuth2.BaseURL, logger)
	// Note: Do NOT call SetHTTPClient here - OAuth2Client already has proxy disabled for token refresh

	// Initialize enhanced token management service
	tokenManagementService := service.NewTokenManagementService(tokenRepo, oauth2Client, logger)

	// Initialize token rotation manager
	tokenRotationManager := service.NewTokenRotationManager(tokenRepo, tokenManagementService, logger)

	// Start token rotation monitoring
	if err := tokenRotationManager.StartMonitoring(ctx); err != nil {
		logger.Error("Failed to start token rotation monitoring", "error", err)
	} else {
		logger.Info("Token rotation monitoring started")
	}

	defer tokenRotationManager.StopMonitoring()

	// Utils initialization
	sanitizer := utils.NewSanitizer()

	// Enhanced Token Serviceを使用したInoreaderサービス
	inoreaderClient := service.NewInoreaderClient(oauth2Client, logger, sanitizer)

	// api_usage_tracking_enabled: real Postgres-backed usage counters for the
	// 100-req/day Zone1 limit, replacing the test mock that used to be DI'd here
	// (which silently no-op'd tracking and pulled a test-only package into prod).
	logger.Info("api_usage_tracking_enabled", "table", "api_usage_tracking")
	apiUsageRepo := repository.NewPostgreSQLAPIUsageRepository(pool, logger)
	// Connect InoreaderService to RemoteTokenService (via TokenProvider interface)
	inoreaderService := service.NewInoreaderService(inoreaderClient, apiUsageRepo, tokenProvider, logger)

	subscriptionSyncService := service.NewSubscriptionSyncService(inoreaderService, subscriptionRepo, syncStateRepo, logger)
	rateLimitManager := service.NewRateLimitManager(nil, logger)

	// Initialize service layer with rotation support
	articleFetchService := service.NewArticleFetchService(
		inoreaderService,
		articleRepo,
		syncStateRepo,
		subscriptionRepo,
		logger,
	)

	// Initialize handler layer (keep legacy handler for subscription sync)
	articleFetchHandler := handler.NewArticleFetchHandler(
		inoreaderService,
		subscriptionSyncService,
		rateLimitManager,
		articleRepo,
		syncStateRepo,
		logger,
	)

	scheduleHandler := handler.NewScheduleHandler(articleFetchHandler, articleFetchService, logger)
	scheduleHandler.SetTokenManager(tokenManagementService)

	// The scheduler that actually drives ticker-based fetch/refresh (started
	// below). Constructed here — ahead of the Admin API trigger endpoints —
	// so those endpoints can call into it directly (TriggerFetchNow /
	// TriggerRefreshNow) instead of ScheduleHandler's own trigger methods.
	// The two used to have no shared guard: an admin-triggered fetch could
	// run concurrently with the ticker-driven fetch and double-consume the
	// 100 req/day Inoreader quota.
	inoreaderScheduler := scheduler.NewScheduler(
		syncStateRepo,
		subscriptionSyncService,
		articleFetchService,
		logger,
	)

	// Add job result callback for monitoring
	scheduleHandler.AddJobResultCallback(func(result *handler.JobResult) {
		logger.Info("Scheduled job completed",
			"job_type", result.JobType,
			"success", result.Success,
			"duration", result.Duration,
			"error", result.Error)
	})

	// セキュリティコンポーネントの初期化
	authenticator := security.NewKubernetesAuthenticator(logger)
	rateLimiter := security.NewMemoryRateLimiter(5, logger) // 5 requests per hour
	inputValidator := security.NewOWASPInputValidator()
	metricsCollector := &SimpleAdminAPIMetricsCollector{logger: logger}

	// Admin API用のトークンマネージャーアダプター (passed from main)
	// tokenManagerAdapter is already initialized in main

	// Admin APIハンドラー作成
	adminAPIHandler := handler.NewAdminAPIHandler(
		tokenManagerAdapter,
		authenticator,
		rateLimiter,
		inputValidator,
		logger,
		metricsCollector,
	)

	// Admin APIサーバー設定
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/admin/oauth2/refresh-token", adminAPIHandler.HandleRefreshTokenUpdate)
	adminMux.HandleFunc("/admin/oauth2/token-status", adminAPIHandler.HandleTokenStatus)

	// /admin/health surfaces ingestion staleness + token availability so the silent
	// failure mode observed during the 2026-05-05 to 2026-05-08 outage becomes pollable.
	healthAdapter := newIngestionHealthAdapter(inoreaderService, tokenProvider)
	healthHandler := handler.NewHealthHandler(healthAdapter, healthRealClock{}, 1800, logger)
	adminMux.HandleFunc("/admin/health", healthHandler.HandleHealth)

	// Manual trigger endpoints for testing - gated behind the same
	// authenticator/rate-limiter/HTTPS-enforcement chain as the rest of the
	// Admin API so an unauthenticated network peer can't exhaust the Inoreader
	// API quota by hitting these directly.
	adminMux.HandleFunc("/admin/trigger/article-fetch", adminAPIHandler.RequireAdmin("/admin/trigger/article-fetch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		logger.Info("Manual article fetch triggered via Admin API")
		err := inoreaderScheduler.TriggerFetchNow()
		if err != nil {
			logger.Error("Failed to trigger article fetch", "error", err)
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "triggered",
			"message":   "Article fetch (rotation) has been triggered manually",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		}); err != nil {
			logger.Error("Failed to encode article-fetch trigger response", "error", err)
		}
	}))

	adminMux.HandleFunc("/admin/trigger/subscription-sync", adminAPIHandler.RequireAdmin("/admin/trigger/subscription-sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		logger.Info("Manual subscription sync triggered via Admin API")
		err := inoreaderScheduler.TriggerRefreshNow()
		if err != nil {
			logger.Error("Failed to trigger subscription sync", "error", err)
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "triggered",
			"message":   "Subscription sync has been triggered manually",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		}); err != nil {
			logger.Error("Failed to encode subscription-sync trigger response", "error", err)
		}
	}))

	adminServer := &http.Server{
		Addr:         ":8080",
		Handler:      adminMux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Admin APIサーバーをgoroutineで起動
	var adminServerWG sync.WaitGroup
	adminServerWG.Add(1)
	go func() {
		defer adminServerWG.Done()
		logger.Info("Starting Admin API server", "address", ":8080")
		if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Admin API server failed", "error", err)
		}
	}()

	// シャットダウン時のクリーンアップ処理
	defer func() {
		logger.Info("Shutting down Admin API server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := adminServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("Failed to shutdown Admin API server gracefully", "error", err)
		}
		adminServerWG.Wait()
	}()

	// Start the dual schedule processing
	logger.Info("Starting dual schedule processing",
		"subscription_sync_interval", "12h",
		"article_fetch_interval", "30m",
		"admin_api_address", ":8080")

	// Use Default Config (16m fetch, 24h refresh)
	schedulerConfig := scheduler.DefaultConfig()
	inoreaderScheduler.Start(schedulerConfig)

	// Register shutdown hook for scheduler
	defer inoreaderScheduler.Stop()

	// scheduleHandler's own ticker-based scheduling (Start) stays unused —
	// inoreaderScheduler above is the sole driver of ticker-based fetch and
	// refresh, and Admin API triggers now go through inoreaderScheduler's
	// TriggerFetchNow/TriggerRefreshNow too, so there is exactly one
	// scheduling authority instead of two independently racing ones.

	// サービス状態の定期ログ（頻度を30分に削減してAPI呼び出しを減らす）
	statusTicker := time.NewTicker(30 * time.Minute)
	defer statusTicker.Stop()
	go func() {
		for {
			select {
			case <-statusTicker.C:
				// Only log connection status to auth-token-manager
				_, err := tokenRepo.GetCurrentToken(ctx)
				isHealthy := err == nil
				logger.Info("Token service status",
					"is_healthy", isHealthy,
					"source", "remote-auth-token-manager")
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for context cancellation or termination signal
	logger.Info("Dual schedule processing started successfully - running indefinitely")
	<-ctx.Done()

	logger.Info("Shutting down dual schedule processing")
	scheduleHandler.Stop()

	return nil
}
