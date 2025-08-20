package rest

import (
	"log/slog"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"auth-service/app/port"
	"auth-service/app/rest/handlers"
	custommw "auth-service/app/rest/middleware"
	"auth-service/app/utils/security"
)

// RouterConfig holds router configuration
type RouterConfig struct {
	Logger          *slog.Logger
	AuthUsecase     port.AuthUsecase
	UserUsecase     port.UserUsecase
	SessionUsecase  port.SessionUsecase
	KratosClient    port.KratosClient
	EnableDebug     bool
	EnableMetrics   bool
}

// NewRouter creates and configures the Echo router
func NewRouter(config RouterConfig) *echo.Echo {
	// Create Echo instance
	e := echo.New()

	// Configure Echo
	e.HideBanner = true
	e.Debug = config.EnableDebug

	// Create handlers
	authHandler := handlers.NewAuthHandler(config.AuthUsecase, config.Logger)
	userHandler := handlers.NewUserHandler(config.UserUsecase, config.Logger)
	healthHandler := handlers.NewHealthHandler(config.Logger)
	debugHandler := handlers.NewDebugHandler(config.KratosClient)

	// Create middleware
	authMiddleware := custommw.NewAuthMiddleware(config.AuthUsecase, config.Logger)
	csrfMiddleware := custommw.NewCSRFMiddleware(config.AuthUsecase, config.Logger)
	
	// Create security components
	rateLimiter := custommw.NewRateLimiter()
	ids := security.NewIDS(config.Logger)

	// Global middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(custommw.DefaultCORS())

	// Enhanced security middleware
	e.Use(custommw.SecurityHeaders())
	e.Use(rateLimiter.RateLimit())
	
	// IDS middleware - Phase 6.0.2: 段階的脅威レベル対応
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ip := c.RealIP()
			userAgent := c.Request().Header.Get("User-Agent")
			path := c.Request().URL.Path
			
			// Body reading for IDS (simplified for now)
			body := ""
			
			// 段階的脅威レベル判定
			threatLevel := ids.AnalyzeRequest(c.Request().Context(), ip, userAgent, path, body)
			
			switch threatLevel {
			case security.ThreatLevelSafe:
				// 安全 - 通常処理継続
				return next(c)
			case security.ThreatLevelSuspect:
				// 疑わしい - ログのみ、ブロックしない
				config.Logger.Warn("Suspicious activity detected",
					"ip", ip,
					"user_agent", userAgent,
					"path", path)
				return next(c)
			case security.ThreatLevelDangerous:
				// 危険 - レート制限
				return c.JSON(429, map[string]interface{}{
					"error": "Rate limited due to suspicious activity",
					"code":  "RATE_LIMITED",
					"details": "Please reduce request frequency",
				})
			case security.ThreatLevelMalicious:
				// 悪意のある - 完全ブロック
				return c.JSON(403, map[string]interface{}{
					"error": "Access denied by security policy",
					"code":  "SECURITY_VIOLATION",
					"details": "Request blocked due to malicious pattern detection",
				})
			default:
				// 未知のレベル - 安全側に倒す
				return next(c)
			}
		}
	})

	// Add custom middleware for request logging
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "method=${method}, uri=${uri}, status=${status}, latency=${latency_human}, error=${error}\n",
	}))

	// API versioning
	v1 := e.Group("/v1")

	// Health endpoints (no auth required)
	health := v1.Group("/health")
	health.GET("", healthHandler.HealthCheck)
	v1.GET("/ready", healthHandler.ReadinessCheck)
	v1.GET("/live", healthHandler.LivenessCheck)

	// Authentication endpoints
	auth := v1.Group("/auth")

	// Public auth endpoints (no auth required)
	// Browser-compatible GET endpoints (following Ory Kratos specification)
	auth.GET("/login/initiate", authHandler.InitiateLoginFlow)
	auth.GET("/login/:flowId", authHandler.GetLoginFlow) // TODO: Implement GetLoginFlow handler
	
	// Legacy POST endpoints (maintained for backward compatibility)
	auth.POST("/login", authHandler.InitiateLogin)
	auth.POST("/login/:flowId", authHandler.CompleteLogin)
	auth.POST("/register", authHandler.InitiateRegistration)
	auth.POST("/register/:flowId", authHandler.CompleteRegistration)
	
	// 🚀 X26 PERMANENT FIX: CSRF endpoint must be public for session initialization
	// CSRF tokens are required BEFORE authentication, so they cannot require auth
	auth.POST("/csrf", authHandler.GenerateCSRFToken)

	// Protected auth endpoints (require authentication)
	authProtected := auth.Group("")
	authProtected.Use(authMiddleware.RequireAuth())
	authProtected.POST("/logout", authHandler.Logout, csrfMiddleware.RequireCSRF())
	authProtected.POST("/refresh", authHandler.RefreshSession, csrfMiddleware.RequireCSRF())
	authProtected.POST("/csrf/validate", authHandler.ValidateCSRFToken)

	// Session validation endpoint (for other services) - memo.md Phase 2.1
	auth.GET("/validate", authHandler.Validate)

	// User endpoints
	user := v1.Group("/user")
	user.Use(authMiddleware.RequireAuth())

	// User profile endpoints
	user.GET("/profile", userHandler.GetProfile)
	user.PUT("/profile", userHandler.UpdateProfile, csrfMiddleware.RequireCSRF())

	// Admin user management endpoints
	adminUser := user.Group("")
	adminUser.Use(authMiddleware.RequireAdmin())
	adminUser.GET("", userHandler.ListUsers)
	adminUser.POST("", userHandler.CreateUser, csrfMiddleware.RequireCSRF())
	adminUser.GET("/:userId", userHandler.GetUserByID)
	adminUser.DELETE("/:userId", userHandler.DeleteUser, csrfMiddleware.RequireCSRF())

	// Metrics endpoint (if enabled)
	if config.EnableMetrics {
		// TODO: Add Prometheus metrics endpoint
		// e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))
	}

	// 🔍 Debug endpoints (only in debug mode)
	if config.EnableDebug {
		debug := v1.Group("/debug")
		
		// CSRF diagnostic endpoints
		debug.GET("/csrf/:flowId", debugHandler.GetCSRFDiagnostic)
		debug.GET("/csrf/health", debugHandler.GetCSRFHealth)
		
		// Legacy debug endpoints
		debug.GET("/registration-flow", debugHandler.DiagnoseRegistrationFlow)
		
		config.Logger.Info("🔧 Debug endpoints enabled", 
			"endpoints", []string{
				"/v1/debug/csrf/:flowId", 
				"/v1/debug/csrf/health",
				"/v1/debug/registration-flow",
			})
	}

	return e
}

// SetupRoutes sets up all routes for the application
func SetupRoutes(e *echo.Echo, config RouterConfig) {
	// This function can be used for additional route setup if needed
	// Currently, all routes are set up in NewRouter
}

// RegisterCustomMiddleware registers custom middleware
func RegisterCustomMiddleware(e *echo.Echo, config RouterConfig) {
	// Create middleware instances
	authMiddleware := custommw.NewAuthMiddleware(config.AuthUsecase, config.Logger)
	csrfMiddleware := custommw.NewCSRFMiddleware(config.AuthUsecase, config.Logger)

	// Store middleware in context for use by specific routes
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("auth_middleware", authMiddleware)
			c.Set("csrf_middleware", csrfMiddleware)
			return next(c)
		}
	})
}