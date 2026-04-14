package bootstrap

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	connectv2 "pre-processor/connect/v2"
	appmiddleware "pre-processor/middleware"
	"pre-processor/tlsutil"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

const (
	httpPort    = "9200" // Default HTTP port for API
	connectPort = "9202" // Default Connect-RPC port
)

// NewHTTPServer creates and configures the Echo HTTP server.
func NewHTTPServer(deps *Dependencies, otelEnabled bool, otelServiceName string) *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	// Custom error handler for consistent error responses
	e.HTTPErrorHandler = appmiddleware.CustomHTTPErrorHandler(deps.Logger)

	// Add OpenTelemetry tracing middleware
	if otelEnabled {
		e.Use(otelecho.Middleware(otelServiceName))
		e.Use(appmiddleware.OTelStatusMiddleware())
	}

	// Middleware
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		Skipper: func(c echo.Context) bool {
			path := c.Request().URL.Path
			return path == "/health" || path == "/api/v1/health"
		},
		LogMethod:  true,
		LogURI:     true,
		LogStatus:  true,
		LogLatency: true,
		LogError:   true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			ctx := c.Request().Context()
			deps.Logger.InfoContext(ctx, "HTTP request completed",
				"method", v.Method,
				"uri", v.URI,
				"status", v.Status,
				"latency", v.Latency,
				"error", v.Error)
			return nil
		},
	}))
	e.Use(middleware.Recover())
	// No CORS: pre-processor is service-to-service only (X-Service-Token required)
	// and must not be callable from a browser origin.

	// Health endpoint stays unauthenticated so liveness probes keep working.
	e.GET("/api/v1/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "healthy"})
	})

	// Summarize endpoints are service-to-service only and require X-Service-Token.
	serviceAuth := appmiddleware.NewServiceAuthMiddleware(deps.Logger)
	api := e.Group("/api/v1")
	// Cap request bodies: article content is already whitespace-collapsed by
	// alt-backend, so 1 MiB comfortably covers real inputs while rejecting
	// DoS payloads before they reach the summarize pipeline.
	api.Use(middleware.BodyLimit("1M"))
	api.Use(serviceAuth.RequireServiceAuth())
	api.POST("/summarize", deps.SummarizeHandler.HandleSummarize)
	api.POST("/summarize/stream", deps.SummarizeHandler.HandleStreamSummarize)
	api.POST("/summarize/queue", deps.SummarizeHandler.HandleSummarizeQueue)
	api.GET("/summarize/status/:job_id", deps.SummarizeHandler.HandleSummarizeStatus)

	return e
}

// StartHTTPServer starts the HTTP server in a goroutine.
func StartHTTPServer(e *echo.Echo, log *slog.Logger) {
	go func() {
		port := os.Getenv("HTTP_PORT")
		if port == "" {
			port = httpPort
		}
		addr := fmt.Sprintf(":%s", port)
		log.Info("Starting HTTP server", "port", port)
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Error("HTTP server error", "error", err)
		}
	}()
}

// StartConnectServer starts the Connect-RPC server in a goroutine. When
// MTLS_LISTEN=true, an additional HTTPS listener on MTLS_PORT (default 9443)
// is started alongside the plaintext listener.
func StartConnectServer(deps *Dependencies) {
	connectHandler := connectv2.CreateConnectServer(deps.APIRepo, deps.SummaryRepo, deps.ArticleRepo, deps.JobRepo, deps.Logger)
	go func() {
		port := os.Getenv("CONNECT_PORT")
		if port == "" {
			port = connectPort
		}
		addr := fmt.Sprintf(":%s", port)
		deps.Logger.Info("Starting Connect-RPC server", "port", port)
		server := &http.Server{
			Addr:        addr,
			Handler:     connectHandler,
			ReadTimeout: 30 * time.Second,
			// WriteTimeout is intentionally 0 (disabled) for server streaming RPCs.
			// Go's http.Server WriteTimeout applies to the ENTIRE stream duration,
			// not per-message. LLM inference can take 30-90+ seconds before the first
			// token, so any fixed timeout would kill streaming connections.
			// Per-RPC deadlines are handled via context.WithTimeout instead.
			WriteTimeout: 0,
			IdleTimeout:  120 * time.Second,
		}
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			deps.Logger.Error("Connect-RPC server error", "error", err)
		}
	}()

	if os.Getenv("MTLS_LISTEN") == "true" {
		mtlsPort := os.Getenv("MTLS_PORT")
		if mtlsPort == "" {
			mtlsPort = "9443"
		}
		tlsCfg, err := tlsutil.LoadServerConfig(
			os.Getenv("MTLS_CERT_FILE"),
			os.Getenv("MTLS_KEY_FILE"),
			os.Getenv("MTLS_CA_FILE"),
			tlsutil.OptionsFromEnv()...,
		)
		if err != nil {
			deps.Logger.Error("mTLS listener config failed, aborting startup (fail-closed)", "error", err)
			os.Exit(1)
		}
		mtlsServer := tlsutil.NewMTLSHTTPServer(":"+mtlsPort, tlsCfg, connectHandler)
		go func() {
			deps.Logger.Info("Starting mTLS HTTPS listener", "port", mtlsPort)
			if err := mtlsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				deps.Logger.Error("mTLS HTTPS listener error", "error", err)
			}
		}()
	}
}
