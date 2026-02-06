package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	adapterhandler "auth-hub/internal/adapter/handler"
	"auth-hub/internal/adapter/gateway"
	infracache "auth-hub/internal/infrastructure/cache"
	infratoken "auth-hub/internal/infrastructure/token"
	"auth-hub/internal/usecase"

	"auth-hub/config"
	appmiddleware "auth-hub/middleware"
	"auth-hub/utils/logger"
	"auth-hub/utils/otel"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"golang.org/x/sync/errgroup"
)

func main() {
	// Handle healthcheck subcommand (for Docker healthcheck in distroless image)
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(); err != nil {
			fmt.Fprintf(os.Stderr, "Healthcheck failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Initialize OpenTelemetry
	otelCfg := otel.ConfigFromEnv()
	otelShutdown, err := otel.InitProvider(ctx, otelCfg)
	if err != nil {
		slog.Warn("failed to initialize OpenTelemetry, continuing without tracing", "error", err)
		otelCfg.Enabled = false
	}

	// Initialize structured logger
	logger.Init(otelCfg.Enabled)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.ErrorContext(ctx, "failed to load configuration", "error", err)
		os.Exit(1)
	}

	slog.InfoContext(ctx, "configuration loaded",
		"kratos_url", cfg.KratosURL,
		"port", cfg.Port,
		"cache_ttl", cfg.CacheTTL)

	// Infrastructure
	sessionCache := infracache.NewSessionCache(cfg.CacheTTL)
	kratosGateway := gateway.NewKratosGateway(cfg.KratosURL, cfg.KratosAdminURL, 5*time.Second)
	jwtIssuer := infratoken.NewJWTIssuer(infratoken.JWTConfig{
		Secret:   cfg.BackendTokenSecret,
		Issuer:   cfg.BackendTokenIssuer,
		Audience: cfg.BackendTokenAudience,
		TTL:      cfg.BackendTokenTTL,
	})
	csrfGenerator := infratoken.NewHMACCSRFGenerator(cfg.CSRFSecret)

	// Usecases
	validateUC := usecase.NewValidateSession(kratosGateway, sessionCache, slog.Default())
	sessionUC := usecase.NewGetSession(kratosGateway, sessionCache, jwtIssuer, slog.Default())
	csrfUC := usecase.NewGenerateCSRF(kratosGateway, csrfGenerator, slog.Default())
	systemUserUC := usecase.NewGetSystemUser(kratosGateway, slog.Default())

	// Handlers
	validateHandler := adapterhandler.NewValidateHandler(validateUC)
	sessionHandler := adapterhandler.NewSessionHandler(sessionUC, cfg.AuthSharedSecret)
	csrfHandler := adapterhandler.NewCSRFHandler(csrfUC)
	healthHandler := adapterhandler.NewHealthHandler()
	internalHandler := adapterhandler.NewInternalHandler(systemUserUC)

	// Setup Echo server
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Security middleware
	e.Use(appmiddleware.SecurityHeaders())

	// OpenTelemetry tracing
	if otelCfg.Enabled {
		e.Use(otelecho.Middleware(otelCfg.ServiceName))
		e.Use(appmiddleware.OTelStatusMiddleware())
	}

	// Request logging
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		Skipper: func(c echo.Context) bool {
			return c.Request().URL.Path == "/health"
		},
		LogStatus:   true,
		LogURI:      true,
		LogError:    true,
		LogMethod:   true,
		LogLatency:  true,
		HandleError: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			rctx := c.Request().Context()
			if v.Error == nil {
				slog.InfoContext(rctx, "request completed",
					"method", v.Method,
					"uri", v.URI,
					"status", v.Status,
					"latency_ms", v.Latency.Milliseconds())
			} else {
				slog.ErrorContext(rctx, "request failed",
					"method", v.Method,
					"uri", v.URI,
					"status", v.Status,
					"latency_ms", v.Latency.Milliseconds(),
					"error", v.Error.Error())
			}
			return nil
		},
	}))

	e.Use(middleware.Recover())

	// Rate limiters per endpoint group
	validateRL := appmiddleware.NewRateLimiter(100.0/60.0, 10) // 100 req/min
	sessionRL := appmiddleware.NewRateLimiter(30.0/60.0, 5)    // 30 req/min
	csrfRL := appmiddleware.NewRateLimiter(10.0/60.0, 3)       // 10 req/min
	internalRL := appmiddleware.NewRateLimiter(10.0/60.0, 3)   // 10 req/min

	// Public routes
	e.GET("/validate", validateHandler.Handle, validateRL.Middleware())
	e.GET("/session", sessionHandler.Handle, sessionRL.Middleware())
	e.POST("/csrf", csrfHandler.Handle, csrfRL.Middleware())
	e.GET("/health", healthHandler.Handle)

	// Internal routes (protected by shared secret)
	internalGroup := e.Group("/internal",
		internalRL.Middleware(),
	)
	if cfg.AuthSharedSecret != "" {
		internalGroup.Use(appmiddleware.InternalAuth(cfg.AuthSharedSecret))
	}
	internalGroup.GET("/system-user", internalHandler.HandleSystemUser)

	// Start server with errgroup for graceful shutdown
	address := fmt.Sprintf(":%s", cfg.Port)
	slog.InfoContext(ctx, "starting auth-hub server", "address", address)

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := e.Start(address); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	g.Go(func() error {
		<-gCtx.Done()
		slog.Info("shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return e.Shutdown(shutdownCtx)
	})

	g.Go(func() error {
		<-gCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return otelShutdown(shutdownCtx)
	})

	if err := g.Wait(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("shutdown error", "error", err)
		os.Exit(1)
	}

	slog.Info("server exited properly")
}

// runHealthcheck performs a health check against the local server.
func runHealthcheck() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8888"
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/health", port))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned status: %d", resp.StatusCode)
	}
	return nil
}
