package di

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"auth-service/app/config"
	"auth-service/app/driver/kratos"
	"auth-service/app/driver/postgres"
	"auth-service/app/gateway"
	"auth-service/app/port"
	"auth-service/app/rest"
	"auth-service/app/usecase"
)

// Container holds all dependencies for the application
type Container struct {
	Config *config.Config
	Logger *slog.Logger
	
	// Drivers
	DB          *postgres.DB
	KratosClient *kratos.Client
	
	// Gateways
	AuthGateway    port.AuthGateway
	// UserGateway    port.UserGateway
	// SessionGateway port.SessionGateway
	
	// Usecases
	AuthUsecase    port.AuthUsecase
	// UserUsecase    port.UserUsecase
	// SessionUsecase port.SessionUsecase
}

// NewContainer creates and initializes a new dependency injection container
func NewContainer(cfg *config.Config, logger *slog.Logger) (*Container, error) {
	container := &Container{
		Config: cfg,
		Logger: logger,
	}

	// Initialize drivers
	var err error
	
	// Initialize database connection
	container.DB, err = postgres.NewConnection(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	
	// Initialize Kratos client
	container.KratosClient, err = kratos.NewClient(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Kratos client: %w", err)
	}
	
	// Initialize repositories
	authRepository := postgres.NewAuthRepository(container.DB.Pool(), logger)
	
	// Initialize gateways
	kratosClientAdapter := kratos.NewKratosClientAdapter(container.KratosClient)
	container.AuthGateway = gateway.NewAuthGateway(kratosClientAdapter, logger)
	
	// Initialize usecases
	container.AuthUsecase = usecase.NewAuthUseCase(authRepository, container.AuthGateway)
	
	// TODO: Initialize remaining usecases when implementations are complete
	// container.UserUsecase = usecase.NewUserUsecase(...)
	// container.SessionUsecase = usecase.NewSessionUsecase(...)

	logger.Info("Container initialized with full dependency stack")

	return container, nil
}

// CreateRouter creates and returns a fully configured Echo router
func (c *Container) CreateRouter() *echo.Echo {
	// Create router configuration
	routerConfig := rest.RouterConfig{
		Logger:          c.Logger,
		AuthUsecase:     c.AuthUsecase,
		UserUsecase:     nil, // TODO: Add when implementation is complete
		SessionUsecase:  nil, // TODO: Add when implementation is complete
		EnableDebug:     c.Config.LogLevel == "debug",
		EnableMetrics:   c.Config.EnableMetrics,
	}

	// Create and return the full router
	router := rest.NewRouter(routerConfig)
	
	c.Logger.Info("Full API router created")
	return router
}

// SetupRoutes configures all routes for the Echo server (legacy method)
func (c *Container) SetupRoutes(e *echo.Echo) {
	// For backwards compatibility, just set up basic health routes
	e.GET("/health", c.healthHandler)
	e.GET("/health/ready", c.readyHandler)
	e.GET("/health/live", c.liveHandler)

	c.Logger.Info("Basic health check routes configured")
}

// Health check handlers

func (c *Container) healthHandler(ctx echo.Context) error {
	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"status":    "healthy",
		"service":   "auth-service",
		"version":   getVersion(),
		"timestamp": time.Now().UTC(),
	})
}

func (c *Container) readyHandler(ctx echo.Context) error {
	// Check database connectivity
	if err := c.DB.HealthCheck(ctx.Request().Context()); err != nil {
		return ctx.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"status": "not_ready",
			"error":  "database connection failed",
		})
	}
	
	// Check Kratos connectivity
	if err := c.KratosClient.HealthCheck(ctx.Request().Context()); err != nil {
		return ctx.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"status": "not_ready",
			"error":  "kratos connection failed",
		})
	}

	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"status": "ready",
	})
}

func (c *Container) liveHandler(ctx echo.Context) error {
	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"status": "alive",
	})
}

// Close closes all resources
func (c *Container) Close() error {
	// Close database connection
	if c.DB != nil {
		c.DB.Close()
	}
	
	// Note: Kratos client doesn't need explicit cleanup
	
	c.Logger.Info("Container closed successfully")
	return nil
}

// Helper functions
func getVersion() string {
	// This should match the version function in main.go
	return "dev"
}
