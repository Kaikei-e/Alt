// Command datahub serves alt-backend's service-to-service surface:
// BackendInternalService over Connect-RPC plus the /v1/internal REST routes
// that pre-processor and rag-orchestrator call.
//
// It has exactly one externally reachable listener and that listener speaks
// mutual TLS. There is no plaintext fallback, no MTLS_LISTEN flag to turn
// verification off, and no route from this binary into the browser-facing API
// — those handlers are not compiled in.
package main

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"

	"alt/config"
	datahubconnect "alt/connect/v2/datahub"
	dataplanerest "alt/dataplane/rest"
	"alt/di"
	"alt/internal/bootstrap"
	"alt/middleware"
	"alt/tlsutil"

	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
)

const serviceName = "alt-data-hub"

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

	if err := config.ValidateDataHubConfig(cfg); err != nil {
		log.ErrorContext(ctx, "datahub config invalid", "error", err)
		os.Exit(1)
	}
	dcfg, err := config.LoadDataHubConfig()
	if err != nil {
		log.ErrorContext(ctx, "datahub listener config invalid", "error", err)
		os.Exit(1)
	}
	healthAddr, err := config.LoadDataHubHealthAddr()
	if err != nil {
		log.ErrorContext(ctx, "datahub health listener config invalid", "error", err)
		os.Exit(1)
	}

	container := di.NewDataHubComponents(rt.Pool, cfg)

	// Client authentication is a constant here, not a setting. The listener it
	// replaced read MTLS_CLIENT_AUTH and fell back to tls.NoClientCert — TLS
	// that encrypts but verifies nobody — whenever the variable was unset,
	// which made "mTLS boundary" a claim the deployment might not honour.
	tlsCfg, err := tlsutil.LoadServerConfig(
		dcfg.CertFile, dcfg.KeyFile, dcfg.CAFile,
		tlsutil.WithClientAuth(tls.RequireAndVerifyClientCert),
		tlsutil.WithAllowedPeers(dcfg.AllowedPeers...),
	)
	if err != nil {
		log.ErrorContext(ctx, "datahub mTLS config failed, refusing to start", "error", err)
		os.Exit(1)
	}

	log.InfoContext(ctx, "datahub_listener.wiring",
		"addr", dcfg.ListenAddr,
		"client_auth", "require_and_verify",
		"allowed_peers", dcfg.AllowedPeers,
		"plaintext_listeners", 0,
		"surfaces", "BackendInternalService,/v1/internal,/health",
	)

	internalEcho := newInternalEcho(container)
	handler := dataHubHandler(datahubconnect.CreateServer(container, cfg, log), internalEcho)
	// The allowlist is re-checked per request, not only at handshake time: a
	// TLS terminator moving in front of this process would otherwise silently
	// retire the handshake-time check. See middleware.RequirePeerIdentity.
	handler = middleware.RequirePeerIdentity(dcfg.AllowedPeers, handler)
	handler = middleware.PeerIdentityHTTPMiddleware(handler)

	srv := tlsutil.NewMTLSHTTPServer(dcfg.ListenAddr, tlsCfg, handler)

	// The container probe gets its own loopback listener rather than a client
	// certificate. It serves /health and /metrics and carries no data-plane
	// surface, so it is not a second entrance to anything.
	healthSrv := bootstrap.NewServiceServer(healthAddr, newHealthHandler(rt.MetricsHandler), cfg)
	log.InfoContext(ctx, "datahub_health_listener.wiring",
		"addr", healthAddr, "surfaces", "/health,/metrics", "control", "loopback_bind_only")

	sup := bootstrap.NewSupervisor(log)
	sup.AddServer("mtls", func() error { return srv.ListenAndServeTLS("", "") }, srv.Shutdown)
	sup.AddServer("health", healthSrv.ListenAndServe, healthSrv.Shutdown)
	sup.Start(ctx)

	outcome := sup.Wait(ctx)
	bootstrap.LogMemStats(ctx, log)
	cancel()
	sup.GracefulShutdown(ctx, cfg.Server.WriteTimeout)

	log.Info("datahub stopped", "reason", outcome.Reason, "signal", outcome.Signal, "server", outcome.Server)
}

// newInternalEcho builds the Echo instance carrying /v1/internal/* and
// /health, unchanged from the internal listener it came from.
func newInternalEcho(container *di.DataHubComponents) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.RequestIDMiddleware())
	e.Use(echomiddleware.Recover())
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "healthy", "service": serviceName})
	})
	dataplanerest.RegisterInternalRoutes(e, container)
	return e
}

// newHealthHandler builds the loopback probe surface on an explicit mux —
// never http.DefaultServeMux, which is where net/http/pprof would register
// itself if this binary ever linked it.
func newHealthHandler(metrics http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"` + serviceName + `"}`))
	})
	if metrics != nil {
		mux.Handle("/metrics", metrics)
	}
	return mux
}
