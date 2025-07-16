package rest

import (
	"log/slog"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"auth-service/app/port"
	"auth-service/app/rest/handlers"
	custommw "auth-service/app/rest/middleware"
)

// RouterConfig holds router configuration
type RouterConfig struct {
	Logger          *slog.Logger
	AuthUsecase     port.AuthUsecase
	UserUsecase     port.UserUsecase
	SessionUsecase  port.SessionUsecase
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

	// Create middleware
	authMiddleware := custommw.NewAuthMiddleware(config.AuthUsecase, config.Logger)
	csrfMiddleware := custommw.NewCSRFMiddleware(config.AuthUsecase, config.Logger)

	// Global middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(custommw.DefaultCORS())

	// Add custom middleware for request logging
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "method=${method}, uri=${uri}, status=${status}, latency=${latency_human}, error=${error}\n",
	}))

	// Security headers
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:           31536000,
		HSTSExcludeSubdomains: false,
		HSTSPreloadEnabled:    false,
	}))

	// Rate limiting (basic)
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(20)))

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
	auth.POST("/login", authHandler.InitiateLogin)
	auth.POST("/login/:flowId", authHandler.CompleteLogin)
	auth.POST("/register", authHandler.InitiateRegistration)
	auth.POST("/register/:flowId", authHandler.CompleteRegistration)

	// Protected auth endpoints (require authentication)
	authProtected := auth.Group("")
	authProtected.Use(authMiddleware.RequireAuth())
	authProtected.POST("/logout", authHandler.Logout, csrfMiddleware.RequireCSRF())
	authProtected.POST("/refresh", authHandler.RefreshSession, csrfMiddleware.RequireCSRF())
	authProtected.POST("/csrf", authHandler.GenerateCSRFToken)
	authProtected.POST("/csrf/validate", authHandler.ValidateCSRFToken)

	// Session validation endpoint (for other services)
	auth.GET("/validate", authHandler.ValidateSession)

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