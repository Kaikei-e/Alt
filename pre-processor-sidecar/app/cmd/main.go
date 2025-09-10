package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"pre-processor-sidecar/config"
	"pre-processor-sidecar/driver"
	"pre-processor-sidecar/handler"
	"pre-processor-sidecar/mocks"
	"pre-processor-sidecar/repository"
	"pre-processor-sidecar/security"
	"pre-processor-sidecar/service"

	"encoding/json"

	_ "github.com/lib/pq"
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

	ctx := context.Background()

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

	simpleTokenConfig := service.SimpleTokenConfig{
		ClientID:            os.Getenv("INOREADER_CLIENT_ID"),
		ClientSecret:        os.Getenv("INOREADER_CLIENT_SECRET"),
		InitialAccessToken:  os.Getenv("INOREADER_ACCESS_TOKEN"),
		InitialRefreshToken: os.Getenv("INOREADER_REFRESH_TOKEN"),
		BaseURL:             cfg.OAuth2.BaseURL, // Use OAuth2-specific base URL
		RefreshBuffer:       30 * time.Minute, // API optimized buffer
		CheckInterval:       3 * time.Hour,   // 3時間間隔（8回/日、API制限対応）
		
		// OAuth2 Secret設定 - auth-token-manager連携
		OAuth2SecretName:    cfg.OAuth2SecretName,
		KubernetesNamespace: os.Getenv("KUBERNETES_NAMESPACE"),
		
		// 恒久対応: 自律的Secret再読み込み機能
		EnableSecretWatch:   os.Getenv("ENABLE_SECRET_WATCH") == "true",
	}

	simpleTokenService, err := service.NewSimpleTokenService(simpleTokenConfig, logger)
	if err != nil {
		logger.Error("Failed to create simple token service", "error", err)
		os.Exit(1)
	}

	// Simple Token Serviceを開始
	if err := simpleTokenService.Start(); err != nil {
		logger.Error("Failed to start simple token service", "error", err)
		os.Exit(1)
	}

	// Graceful shutdown設定
	defer func() {
		logger.Info("Shutting down simple token service...")
		simpleTokenService.Stop()
	}()

	// Run in continuous scheduling mode with new token system
	if *scheduleMode {
		logger.Info("Starting in schedule mode as requested by flag")
	}
	if err := runScheduleMode(ctx, cfg, logger, simpleTokenService); err != nil {
		logger.Error("Scheduler failed", "error", err)
		os.Exit(1)
	}
}

func performHealthCheck() {
	// Simple health check for scheduler
	fmt.Println("OK")
	os.Exit(0)
}

func performOAuth2Initialization(cfg *config.Config, logger *slog.Logger) {
	logger.Info("OAuth2 initialization starting", "service", "oauth2-init")

	// Wait for Linkerd proxy initialization
	logger.Info("Waiting for Linkerd proxy initialization...")
	time.Sleep(10 * time.Second)

	// Initialize database connection (Clean Architecture - use config values)
	dbConnectionString := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port,
		cfg.Database.User, cfg.Database.Password,
		cfg.Database.Name, cfg.Database.SSLMode)

	// Create database connection
	logger.Info("Attempting database connection",
		"host", cfg.Database.Host,
		"port", cfg.Database.Port,
		"user", cfg.Database.User,
		"dbname", cfg.Database.Name,
		"sslmode", cfg.Database.SSLMode)

	db, err := sql.Open("postgres", dbConnectionString)
	if err != nil {
		logger.Error("Failed to create database connection", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Test database connection with retry logic
	maxRetries := 3
	for i := 1; i <= maxRetries; i++ {
		if err := db.Ping(); err != nil {
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
func runScheduleMode(ctx context.Context, cfg *config.Config, logger *slog.Logger, simpleTokenService *service.SimpleTokenService) error {
	logger.Info("Initializing dual schedule processing system")

	// Wait for Linkerd proxy initialization
	logger.Info("Waiting for Linkerd proxy initialization...")
	time.Sleep(10 * time.Second)

	// Initialize database connection
	dbConnectionString := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port,
		cfg.Database.User, cfg.Database.Password,
		cfg.Database.Name, cfg.Database.SSLMode)

	logger.Info("Attempting database connection",
		"host", cfg.Database.Host,
		"port", cfg.Database.Port,
		"user", cfg.Database.User,
		"dbname", cfg.Database.Name,
		"sslmode", cfg.Database.SSLMode)

	db, err := sql.Open("postgres", dbConnectionString)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}
	defer db.Close()

	// Test database connection with retry
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		if err := db.PingContext(ctx); err != nil {
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
	articleRepo := repository.NewPostgreSQLArticleRepository(db, logger)
	syncStateRepo := repository.NewPostgreSQLSyncStateRepository(db, logger)
	subscriptionRepo := repository.NewPostgreSQLSubscriptionRepository(db, logger)

	// OAuth2クライアントの作成（Enhanced Token Serviceと同じ設定）
	clientID := os.Getenv("INOREADER_CLIENT_ID")
	clientSecret := os.Getenv("INOREADER_CLIENT_SECRET")
	oauth2Client := driver.NewOAuth2Client(clientID, clientSecret, cfg.OAuth2.BaseURL, logger)
	// Note: Do NOT call SetHTTPClient here - OAuth2Client already has proxy disabled for token refresh

	// Initialize token repository based on configuration
	var tokenRepo repository.OAuth2TokenRepository
	switch cfg.TokenStorageType {
	case "kubernetes_secret":
		k8sRepo, err := repository.NewKubernetesSecretRepository(
			getNamespace(), 
			cfg.OAuth2SecretName, 
			logger,
		)
		if err != nil {
			logger.Warn("Failed to initialize Kubernetes Secret repository, falling back to environment variables", "error", err)
			tokenRepo = repository.NewEnvVarTokenRepository(logger)
		} else {
			logger.Info("Using Kubernetes Secret token repository")
			tokenRepo = k8sRepo
		}
	case "env_var":
		logger.Info("Using environment variable token repository")
		tokenRepo = repository.NewEnvVarTokenRepository(logger)
	default:
		logger.Warn("Unknown token storage type, defaulting to environment variables", "type", cfg.TokenStorageType)
		tokenRepo = repository.NewEnvVarTokenRepository(logger)
	}

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

	// Enhanced Token Serviceを使用したInoreaderサービス
	inoreaderClient := service.NewInoreaderClient(oauth2Client, logger)

	// Create a mock APIUsageRepository since it's not needed
	mockAPIUsageRepo := &mocks.MockAPIUsageRepository{}
	inoreaderService := service.NewInoreaderService(inoreaderClient, mockAPIUsageRepo, simpleTokenService, logger)

	subscriptionSyncService := service.NewSubscriptionSyncService(inoreaderService, subscriptionRepo, logger)
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

	// Admin API用のトークンマネージャーアダプター作成
	tokenManagerAdapter := service.NewSimpleTokenServiceAdapter(simpleTokenService)

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

	// Manual trigger endpoints for testing
	adminMux.HandleFunc("/admin/trigger/article-fetch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		logger.Info("Manual article fetch triggered via Admin API")
		err := scheduleHandler.TriggerArticleFetch()
		if err != nil {
			logger.Error("Failed to trigger article fetch", "error", err)
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "triggered",
			"message":   "Article fetch (rotation) has been triggered manually",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	adminMux.HandleFunc("/admin/trigger/subscription-sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		logger.Info("Manual subscription sync triggered via Admin API")
		err := scheduleHandler.TriggerSubscriptionSync()
		if err != nil {
			logger.Error("Failed to trigger subscription sync", "error", err)
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "triggered",
			"message":   "Subscription sync has been triggered manually",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

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

	if err := scheduleHandler.Start(ctx); err != nil {
		return fmt.Errorf("failed to start schedule handler: %w", err)
	}

	// サービス状態の定期ログ（頻度を30分に削減してAPI呼び出しを減らす）
	statusTicker := time.NewTicker(30 * time.Minute)
	defer statusTicker.Stop()
	go func() {
		for {
			select {
			case <-statusTicker.C:
				status := simpleTokenService.GetServiceStatus()
				logger.Info("Token service status",
					"is_healthy", status.IsHealthy,
					"token_expires_in_seconds", status.TokenStatus.ExpiresInSeconds,
					"consecutive_failures", status.RecoveryStats.ConsecutiveFailures,
					"is_in_recovery_mode", status.RecoveryStats.IsInRecoveryMode)
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
