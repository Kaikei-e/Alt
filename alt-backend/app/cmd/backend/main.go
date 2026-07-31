// Command backend serves alt-backend's user-facing surfaces: the browser REST
// API, the browser Connect-RPC API, and a loopback operator listener carrying
// the admin RPCs.
//
// It deliberately runs no scheduled jobs (cmd/harvester) and serves no
// service-to-service surface (cmd/datahub). Neither is a matter of
// configuration: NewBackendComponents does not build the components those
// surfaces need, so re-adding them here would not compile.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"alt/config"
	connectv2 "alt/connect/v2"
	"alt/di"
	"alt/internal/bootstrap"
	"alt/internal/profiling"
	"alt/middleware"
	"alt/orchestrator/rest"
	altotel "alt/utils/otel"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

const serviceName = "alt-backend"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt := bootstrap.MustBoot(ctx, bootstrap.Options{
		ServiceName: serviceName,
		RequireDB:   true,
	})
	defer rt.Shutdown(ctx)

	cfg := rt.Cfg
	log := rt.Log

	// Rule 9: settle every listener decision before building anything.
	if err := config.ValidateBackendListeners(cfg); err != nil {
		log.ErrorContext(ctx, "backend listener config invalid", "error", err)
		os.Exit(1)
	}
	if err := config.ValidateBackendConfig(cfg); err != nil {
		log.ErrorContext(ctx, "backend config invalid", "error", err)
		os.Exit(1)
	}
	if err := config.RejectBackendMTLSListenerEnv(); err != nil {
		log.ErrorContext(ctx, "stale mtls listener config", "error", err)
		os.Exit(1)
	}
	operatorAddr, err := config.LoadOperatorListenAddr()
	if err != nil {
		log.ErrorContext(ctx, "operator listener config invalid", "error", err)
		os.Exit(1)
	}

	pprofSrv := profiling.Start(ctx, log)
	defer pprofSrv.Close()

	container := di.NewBackendComponents(rt.Pool, cfg)

	otelEnabled := altotel.ConfigFromEnv().Enabled

	// ---- Listener 1: browser-facing REST ----
	e := newRESTEcho(container, cfg, otelEnabled, rt.MetricsHandler)
	rest.RegisterRoutes(ctx, e, container, cfg)
	restSrv := bootstrap.NewRESTServer(fmt.Sprintf(":%d", cfg.Server.Port), e, cfg)

	// ---- Listener 2: browser-facing Connect-RPC ----
	connectSrv := bootstrap.NewConnectServer(
		fmt.Sprintf(":%d", cfg.Server.ConnectPort),
		connectv2.CreateConnectServer(container, cfg, log),
	)

	// ---- Listener 3: operator (loopback) ----
	// KnowledgeHomeAdminService and AdminMonitorService authenticate nobody;
	// the loopback bind is the entire access control, so say so at startup
	// rather than leaving it implied by a compose port mapping (rule 8).
	log.InfoContext(ctx, "operator_listener.wiring",
		"addr", operatorAddr,
		"surfaces", "KnowledgeHomeAdminService,AdminMonitorService,/metrics",
		"auth", "none",
		"control", "loopback_bind_only",
		"admin_monitor_enabled", container.AdminMonitor != nil && container.AdminMonitor.Enabled,
	)
	operatorSrv := bootstrap.NewServiceServer(operatorAddr, newOperatorHandler(container, cfg, log, rt.MetricsHandler), cfg)

	sup := bootstrap.NewSupervisor(log)
	sup.AddServer("rest", func() error { return e.StartServer(restSrv) }, e.Shutdown)
	sup.AddServer("connect", connectSrv.ListenAndServe, connectSrv.Shutdown)
	sup.AddServer("operator", operatorSrv.ListenAndServe, operatorSrv.Shutdown)
	sup.Start(ctx)

	outcome := sup.Wait(ctx)
	bootstrap.LogMemStats(ctx, log)
	cancel()
	sup.GracefulShutdown(ctx, cfg.Server.WriteTimeout)

	log.Info("server stopped", "reason", outcome.Reason, "signal", outcome.Signal, "server", outcome.Server)
}

// newRESTEcho builds the browser-facing Echo instance, unchanged from the
// single-binary main.go: OTel middleware, the error handler that preserves a
// 401 instead of rewriting it to 404, and the Prometheus endpoint.
func newRESTEcho(container *di.ApplicationComponents, cfg *config.Config, otelEnabled bool, metrics http.Handler) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = false

	if otelEnabled {
		e.Use(otelecho.Middleware(serviceName))
		e.Use(middleware.OTelStatusMiddleware())
	}

	// Preserve the original status code: the default handler turned a 401 into
	// a 404, which made an expired session look like a missing route.
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if he, ok := err.(*echo.HTTPError); ok {
			_ = c.JSON(he.Code, map[string]interface{}{
				"error":  http.StatusText(he.Code),
				"detail": he.Message,
			})
			return
		}
		_ = c.JSON(http.StatusInternalServerError, map[string]interface{}{"error": "internal_error"})
	}

	if metrics != nil {
		e.GET("/metrics", echo.WrapHandler(metrics))
	}
	return e
}

// newOperatorHandler serves the admin Connect-RPC services plus /metrics.
//
// /metrics is also still served on the REST listener. That is deliberate
// duplication, not an oversight: the operator listener binds loopback inside
// the container, so a Prometheus server in another container cannot scrape it,
// and silently moving the scrape target would have taken the backend's metrics
// offline. The operator copy exists so an operator shelling into the container
// has the whole operator surface on one address.
func newOperatorHandler(container *di.ApplicationComponents, cfg *config.Config, log *slog.Logger, metrics http.Handler) http.Handler {
	connectHandler := connectv2.CreateOperatorConnectServer(container, cfg, log)
	if metrics == nil {
		return connectHandler
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics)
	mux.Handle("/", connectHandler)
	return mux
}
