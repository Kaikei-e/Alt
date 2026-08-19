// Command backend serves alt-backend's user-facing surfaces: the browser REST
// API, the browser Connect-RPC API, an operator listener carrying the admin
// RPCs, and the ops listener shared with cmd/harvester and cmd/datahub.
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
	"alt/internal/healthdeep"
	"alt/internal/profiling"
	"alt/middleware"
	"alt/orchestrator/rest"
	"alt/shared/driver/datahub_client"
	altotel "alt/utils/otel"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

const serviceName = "alt-backend"

func main() {
	// The healthcheck subcommand runs before anything is booted: compose calls
	// it every interval, and a probe that first opened the database pool would
	// report a healthy process unhealthy whenever a dependency was slow.
	if bootstrap.IsHealthcheckInvocation(os.Args) {
		runHealthcheck()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt := bootstrap.MustBoot(ctx, bootstrap.Options{
		ServiceName: serviceName,
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
	opsAddr, err := config.LoadOpsListenAddr()
	if err != nil {
		log.ErrorContext(ctx, "ops listener config invalid", "error", err)
		os.Exit(1)
	}
	if err := bootstrap.StartEnrollment(ctx, rt, serviceName); err != nil {
		log.ErrorContext(ctx, "pki enrollment failed", "error", err)
		os.Exit(1)
	}

	pprofSrv := profiling.Start(ctx, log)
	defer pprofSrv.Close()

	container := di.NewBackendComponents(cfg)

	otelEnabled := altotel.ConfigFromEnv().Enabled

	// ---- Listener 1: browser-facing REST ----
	e := newRESTEcho(container, cfg, otelEnabled)
	rest.RegisterRoutes(ctx, e, container, cfg)
	restSrv := bootstrap.NewRESTServer(fmt.Sprintf(":%d", cfg.Server.Port), e, cfg)

	// ---- Listener 2: browser-facing Connect-RPC ----
	connectSrv := bootstrap.NewConnectServer(
		fmt.Sprintf(":%d", cfg.Server.ConnectPort),
		connectv2.CreateConnectServer(container, cfg, log),
	)

	// ---- Listener 3: operator ----
	// KnowledgeHomeAdminService and AdminMonitorService authenticate nobody, so
	// the bind address is the entire access control. It defaults to loopback
	// and compose widens it to ":9102" to match the 127.0.0.1:9102:9102
	// publish that altctl uses — docker-proxy dials the container's eth0, which
	// a loopback bind inside the netns does not answer. Because the value can
	// be widened, the process states what it actually bound and how far that
	// reaches (rule 8); the compose publish is what keeps it off other hosts.
	log.InfoContext(ctx, "operator_listener.wiring",
		"addr", operatorAddr,
		"reach", string(config.ListenAddrReach(operatorAddr)),
		"surfaces", "KnowledgeHomeAdminService,AdminMonitorService",
		"auth", "none",
		"admin_monitor_enabled", container.AdminMonitor != nil && container.AdminMonitor.Enabled,
	)
	operatorSrv := bootstrap.NewServiceServer(operatorAddr, newOperatorHandler(container, cfg, log), cfg)

	// ---- Listener 4: ops (/health + /metrics + /health/deep) ----
	dhCfg, err := config.LoadDataHubClientConfig()
	if err != nil {
		log.ErrorContext(ctx, "data-hub client configuration is invalid", "error", err)
		os.Exit(1)
	}
	dhHealthClient, err := datahub_client.NewHTTPClient(dhCfg, healthdeep.DefaultGlobalBudget)
	if err != nil {
		log.ErrorContext(ctx, "data-hub health client failed", "error", err)
		os.Exit(1)
	}
	deep := healthdeep.NewRunner(healthdeep.Config{
		Service: serviceName,
		Checks: []healthdeep.Check{{
			Name:     "datahub",
			Critical: true,
			Probe: func(ctx context.Context) error {
				return datahub_client.Ping(ctx, dhHealthClient, dhCfg.BaseURL)
			},
		}},
	})
	log.InfoContext(ctx, "health_deep_enabled", "path", "/health/deep", "checks", "datahub")
	bootstrap.LogOpsWiring(ctx, log, opsAddr, rt.MetricsHandler != nil, "/health/deep")
	opsSrv := bootstrap.NewOpsServer(opsAddr, bootstrap.NewOpsHandler(serviceName, rt.MetricsHandler, bootstrap.WithDeepHealth(deep.Handler())), cfg)

	sup := bootstrap.NewSupervisor(log)
	sup.AddServer("rest", func() error { return e.StartServer(restSrv) }, e.Shutdown)
	sup.AddServer("connect", connectSrv.ListenAndServe, connectSrv.Shutdown)
	sup.AddServer("operator", operatorSrv.ListenAndServe, operatorSrv.Shutdown)
	sup.AddServer("ops", opsSrv.ListenAndServe, opsSrv.Shutdown)
	sup.Start(ctx)

	outcome := sup.Wait(ctx)
	bootstrap.LogMemStats(ctx, log)
	cancel()
	sup.GracefulShutdown(ctx, cfg.Server.WriteTimeout)

	log.Info("server stopped", "reason", outcome.Reason, "signal", outcome.Signal, "server", outcome.Server)
}

// runHealthcheck self-probes the ops listener and exits. cmd/backend has no
// second listener the probe has to reach: :9000 and :9101 are user-facing and
// the operator listener carries no health route, so a live ops surface is the
// liveness statement.
func runHealthcheck() {
	addr, err := config.LoadOpsListenAddr()
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		os.Exit(1)
	}
	if err := bootstrap.Healthcheck(context.Background(), bootstrap.HealthcheckOptions{OpsAddr: addr}); err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// newRESTEcho builds the browser-facing Echo instance: OTel middleware and the
// error handler that preserves a 401 instead of rewriting it to 404.
//
// /metrics is deliberately absent. It used to sit here, which meant the
// browser-facing listener was also the monitoring surface and the three split
// binaries each exposed metrics somewhere different. It now lives on the ops
// listener alone, and e2e/hurl/alt-backend/03-topology-internal-surface-
// absent.hurl asserts the 404 that this omission produces.
func newRESTEcho(container *di.ApplicationComponents, cfg *config.Config, otelEnabled bool) *echo.Echo {
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

	return e
}

// newOperatorHandler serves the admin Connect-RPC services, and only those.
//
// It used to carry a second copy of /metrics so an operator inside the
// container had everything on one address. That copy is gone: Prometheus
// scrapes the ops listener on :9110 (the same port for all three binaries),
// and keeping a duplicate here meant the admin port had a route whose presence
// nothing tested and whose absence nothing would notice.
func newOperatorHandler(container *di.ApplicationComponents, cfg *config.Config, log *slog.Logger) http.Handler {
	return connectv2.CreateOperatorConnectServer(container, cfg, log)
}
