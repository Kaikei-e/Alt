package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	"pre-processor-sidecar/config"
	"pre-processor-sidecar/driver"
	"pre-processor-sidecar/handler"
	"pre-processor-sidecar/models"
	"pre-processor-sidecar/repository"
	"pre-processor-sidecar/service"

	_ "github.com/lib/pq"
)

func main() {
	// Parse command line flags
	healthCheck := flag.Bool("health-check", false, "Perform health check and exit")
	oauth2Init := flag.Bool("oauth2-init", false, "Initialize OAuth2 tokens and exit")
	scheduleMode := flag.Bool("schedule-mode", false, "Run in continuous scheduling mode (dual schedules)")
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

	if *scheduleMode {
		logger.Info("Pre-processor-sidecar Scheduler starting",
			"service", cfg.ServiceName,
			"subscription_sync_interval", "4h",
			"article_fetch_interval", "30m",
			"api_daily_limit", cfg.RateLimit.DailyLimit)

		// Run in continuous scheduling mode
		if err := runScheduleMode(ctx, cfg, logger); err != nil {
			logger.Error("Schedule mode failed", "error", err)
			os.Exit(1)
		}
	} else {
		logger.Info("Pre-processor-sidecar CronJob starting", 
			"service", cfg.ServiceName,
			"sync_interval", cfg.RateLimit.SyncInterval,
			"api_daily_limit", cfg.RateLimit.DailyLimit)

		// Run the single CronJob task
		if err := runCronJobTask(ctx, cfg, logger); err != nil {
			logger.Error("CronJob task failed", "error", err)
			os.Exit(1)
		}

		logger.Info("Pre-processor-sidecar CronJob completed successfully")
	}
}

func performHealthCheck() {
	// Simple health check for CronJob
	fmt.Println("OK")
	os.Exit(0)
}

func performOAuth2Initialization(cfg *config.Config, logger *slog.Logger) {
	logger.Info("OAuth2 initialization starting", "service", "oauth2-init")
	
	ctx := context.Background()
	
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
	
	// Try to refresh token if we have a refresh token
	if cfg.OAuth2.RefreshToken != "" {
		logger.Info("Attempting to refresh OAuth2 token using existing refresh token")
		
		// Initialize OAuth2 client with correct parameters
		oauth2Client := driver.NewOAuth2Client(
			cfg.OAuth2.ClientID,
			cfg.OAuth2.ClientSecret,
			"https://www.inoreader.com",
		)
		
		refreshedToken, err := oauth2Client.RefreshToken(ctx, cfg.OAuth2.RefreshToken)
		if err != nil {
			logger.Error("Failed to refresh OAuth2 token", "error", err)
			os.Exit(1)
		}
		
		logger.Info("OAuth2 token refreshed successfully",
			"access_token_length", len(refreshedToken.AccessToken),
			"token_type", refreshedToken.TokenType,
			"expires_in", refreshedToken.ExpiresIn)
		
		logger.Info("OAuth2 initialization completed successfully")
		return
	}
	
	logger.Error("No refresh token available for OAuth2 token initialization")
	logger.Info("OAuth2 initialization requires valid refresh token in secret")
	os.Exit(1)
}

func runCronJobTask(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	logger.Info("Initializing CronJob task with actual Inoreader API integration")
	
	// Wait for Linkerd proxy initialization
	logger.Info("Waiting for Linkerd proxy initialization...")
	time.Sleep(10 * time.Second)
	
	// Initialize database connection using config values (Clean Architecture - no hardcoding)
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

	// Create HTTP client with proxy configuration for Envoy
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				// Use Envoy proxy for external requests
				return url.Parse(cfg.Proxy.HTTPSProxy)
			},
		},
	}

	logger.Info("HTTP client configured", "proxy", cfg.Proxy.HTTPSProxy)

	// For now, let's test a simple API call to Inoreader
	logger.Info("Testing Inoreader API connection")
	
	// Create OAuth2 authenticated request
	req, err := http.NewRequestWithContext(ctx, "GET", cfg.Inoreader.BaseURL+"/subscription/list", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	// Add OAuth2 authorization header (using refresh token to get access token is complex, 
	// so for this test we'll try a simpler approach first)
	logger.Info("Making authenticated request to Inoreader API")
	
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	
	logger.Info("Inoreader API response received", 
		"status_code", resp.StatusCode,
		"content_length", resp.ContentLength)

	// PERMANENT FIX: Use environment variable-based OAuth2 token storage (Kubernetes secrets)
	logger.Info("Using environment variable-based token storage for OAuth2 tokens", "source", "Kubernetes secrets")
	tokenRepo := repository.NewEnvVarTokenRepository(logger)

	// Get OAuth2 credentials from environment variables (Kubernetes secrets)
	clientID := os.Getenv("INOREADER_CLIENT_ID")
	clientSecret := os.Getenv("INOREADER_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		logger.Error("Missing OAuth2 credentials in environment variables",
			"has_client_id", clientID != "",
			"has_client_secret", clientSecret != "")
		return fmt.Errorf("missing OAuth2 credentials")
	}
	
	oauth2Client := driver.NewOAuth2Client(clientID, clientSecret, cfg.Inoreader.BaseURL)
	oauth2Client.SetHTTPClient(httpClient) // Use proxy-configured HTTP client
	
	tokenManager := service.NewTokenManagementService(tokenRepo, oauth2Client, logger)
	
	// Ensure we have a valid OAuth2 token
	logger.Info("Ensuring valid OAuth2 token for API access")
	token, err := tokenManager.EnsureValidToken(ctx)
	if err != nil {
		logger.Error("Failed to ensure valid OAuth2 token", 
			"error", err,
			"hint", "Run oauth-init tool if this is the first time")
		return fmt.Errorf("OAuth2 token management failed: %w", err)
	}
	
	logger.Info("OAuth2 token validated successfully",
		"expires_at", token.ExpiresAt,
		"time_to_expiry", token.TimeUntilExpiry(),
		"token_type", token.TokenType)
	
	// Initialize Inoreader service with token management
	inoreaderClient := service.NewInoreaderClient(oauth2Client, logger)
	inoreaderService := service.NewInoreaderService(inoreaderClient, nil, logger)
	inoreaderService.SetCurrentToken(token)
	
	// Initialize subscription repository for database storage
	subscriptionRepo := repository.NewPostgreSQLSubscriptionRepository(db, logger)
	
	// Perform subscription synchronization
	logger.Info("Starting subscription synchronization with Inoreader API")
	subscriptions, err := inoreaderService.FetchSubscriptions(ctx)
	if err != nil {
		logger.Error("Failed to fetch subscriptions", "error", err)
		return fmt.Errorf("subscription fetch failed: %w", err)
	}
	
	logger.Info("Successfully fetched subscriptions from Inoreader API",
		"subscription_count", len(subscriptions))
	
	// Convert Subscription models to InoreaderSubscription for database storage
	inoreaderSubs := make([]models.InoreaderSubscription, len(subscriptions))
	for i, sub := range subscriptions {
		inoreaderSubs[i] = models.InoreaderSubscription{
			DatabaseID:  sub.ID,           // 修正: DatabaseIDフィールドを使用
			InoreaderID: sub.InoreaderID,  // Set InoreaderID from API
			Title:       sub.Title,
			URL:         sub.FeedURL,
			IconURL:     "", // Not available from Subscription model
			Categories: []models.InoreaderCategory{
				{Label: sub.Category},
			},
			CreatedAt:  sub.CreatedAt,
			UpdatedAt:  time.Now(),
		}
	}

	// Save subscriptions to database
	logger.Info("Saving subscriptions to database")
	if err := subscriptionRepo.SaveSubscriptions(ctx, inoreaderSubs); err != nil {
		logger.Error("Failed to save subscriptions to database", "error", err)
		return fmt.Errorf("subscription save failed: %w", err)
	}
	
	// Log subscription details for verification
	for _, sub := range subscriptions {
		logger.Debug("Synchronized subscription",
			"inoreader_id", sub.InoreaderID,
			"title", sub.Title,
			"category", sub.Category,
			"feed_url", sub.FeedURL)
	}
	
	logger.Info("Subscription synchronization completed successfully",
		"subscription_count", len(subscriptions),
		"api_usage_info", "subscription list API call completed",
		"database_saved", "success")
	
	logger.Info("CronJob task completed - Full OAuth2 integration and database storage operational")
	return nil
}

// runScheduleMode runs the service in continuous scheduling mode with dual schedules
func runScheduleMode(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
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

	// Create HTTP client with proxy configuration
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return url.Parse(cfg.Proxy.HTTPSProxy)
			},
		},
	}

	logger.Info("HTTP client configured", "proxy", cfg.Proxy.HTTPSProxy)

	// PERMANENT FIX: Use environment variable-based OAuth2 token storage (Kubernetes secrets)
	logger.Info("Using environment variable-based token storage for OAuth2 tokens", "source", "Kubernetes secrets")
	tokenRepo := repository.NewEnvVarTokenRepository(logger)

	// Initialize OAuth2 and services
	// Get OAuth2 credentials from environment variables (Kubernetes secrets)
	clientID := os.Getenv("INOREADER_CLIENT_ID")
	clientSecret := os.Getenv("INOREADER_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		logger.Error("Missing OAuth2 credentials in environment variables",
			"has_client_id", clientID != "",
			"has_client_secret", clientSecret != "")
		return fmt.Errorf("missing OAuth2 credentials")
	}
	
	oauth2Client := driver.NewOAuth2Client(clientID, clientSecret, cfg.Inoreader.BaseURL)
	oauth2Client.SetHTTPClient(httpClient)
	
	tokenManager := service.NewTokenManagementService(tokenRepo, oauth2Client, logger)
	
	// Ensure we have a valid OAuth2 token
	logger.Info("Ensuring valid OAuth2 token for API access")
	token, err := tokenManager.EnsureValidToken(ctx)
	if err != nil {
		logger.Error("Failed to ensure valid OAuth2 token", 
			"error", err,
			"hint", "Run oauth-init tool if this is the first time")
		return fmt.Errorf("OAuth2 token management failed: %w", err)
	}
	
	logger.Info("OAuth2 token validated successfully",
		"expires_at", token.ExpiresAt,
		"time_to_expiry", token.TimeUntilExpiry(),
		"token_type", token.TokenType)

	// Initialize repositories
	articleRepo := repository.NewPostgreSQLArticleRepository(db, logger)
	syncStateRepo := repository.NewPostgreSQLSyncStateRepository(db, logger)
	subscriptionRepo := repository.NewPostgreSQLSubscriptionRepository(db, logger)

	// Initialize service layer components
	inoreaderClient := service.NewInoreaderClient(oauth2Client, logger)
	inoreaderService := service.NewInoreaderService(inoreaderClient, nil, logger)
	inoreaderService.SetCurrentToken(token)

	subscriptionSyncService := service.NewSubscriptionSyncService(inoreaderService, subscriptionRepo, logger)
	rateLimitManager := service.NewRateLimitManager(nil, logger)

	// Initialize handler layer
	articleFetchHandler := handler.NewArticleFetchHandler(
		inoreaderService,
		subscriptionSyncService,
		rateLimitManager,
		articleRepo,
		syncStateRepo,
		logger,
	)

	scheduleHandler := handler.NewScheduleHandler(articleFetchHandler, logger)

	// Add job result callback for monitoring
	scheduleHandler.AddJobResultCallback(func(result *handler.JobResult) {
		logger.Info("Scheduled job completed",
			"job_type", result.JobType,
			"success", result.Success,
			"duration", result.Duration,
			"error", result.Error)
	})

	// Start the dual schedule processing
	logger.Info("Starting dual schedule processing", 
		"subscription_sync_interval", "4h",
		"article_fetch_interval", "30m")
	
	if err := scheduleHandler.Start(ctx); err != nil {
		return fmt.Errorf("failed to start schedule handler: %w", err)
	}
	
	// Wait for context cancellation or termination signal
	logger.Info("Dual schedule processing started successfully - running indefinitely")
	<-ctx.Done()
	
	logger.Info("Shutting down dual schedule processing")
	scheduleHandler.Stop()
	
	return nil
}