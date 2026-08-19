// Command datahub serves alt-backend's service-to-service surface:
// services.datahub.v1.DataHubService over Connect-RPC, and nothing else.
//
// Two former names for the same capabilities were served alongside it through
// ADR-000954 Wave 2 — services.backend.v1.BackendInternalService and the
// /v1/internal REST routes pre-processor and rag-orchestrator called. Wave 2-C
// removed both once every consumer had moved; the REST pair lives on as the
// GetSystemUser and ListRecentArticles procedures (D6).
//
// It has exactly one externally reachable listener and that listener speaks
// mutual TLS. There is no plaintext fallback, no MTLS_LISTEN flag to turn
// verification off, and no route from this binary into the browser-facing API
// — those handlers are not compiled in.
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"

	"alt/config"
	datahubconnect "alt/connect/v2/datahub"
	"alt/dataplane/connect/datahubapi"
	datahubdi "alt/di/datahub"
	"alt/internal/bootstrap"
	"alt/internal/bootstrap/dbboot"
	"alt/internal/healthdeep"
	"alt/middleware"
	"alt/tlsutil"
)

const serviceName = "alt-data-hub"

func main() {
	// Runs before MustBoot: compose probes this container every 10s, and the
	// probe must not depend on the database pool or the certificate material.
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

	if err := config.ValidateDataHubConfig(cfg); err != nil {
		log.ErrorContext(ctx, "datahub config invalid", "error", err)
		os.Exit(1)
	}
	dcfg, err := config.LoadDataHubConfig()
	if err != nil {
		log.ErrorContext(ctx, "datahub listener config invalid", "error", err)
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

	// The pool is opened here rather than by MustBoot, and by a package the
	// other two binaries do not import. That is what makes "only alt-data-hub
	// can reach alt_db" a property of the link rather than of a boolean —
	// see internal/bootstrap/dbboot and di/import_boundary_test.go.
	pool := dbboot.MustOpen(ctx, rt, serviceName)

	container := datahubdi.NewDataHubComponents(pool, cfg)

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
		"surfaces", "DataHubService,/health,/health/deep",
	)

	deep := newDeepHealthHandler(func(ctx context.Context) error {
		if err := pool.Ping(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return healthdeep.ErrUnavailable
		}
		return nil
	})
	log.InfoContext(ctx, "health_deep_enabled", "path", "/health/deep", "checks", "database")
	handler := dataHubHandler(datahubconnect.CreateServer(container, cfg, log), deep)
	// Transitional (ADR-000955): consumers deployed before the namespace
	// rename still call the retired name. Loud by design (Rule 8) — the log
	// is the evidence the alias is a deliberate wiring state, and its absence
	// after removal is the evidence the transition ended.
	handler = datahubapi.LegacyNamespaceAlias(handler)
	log.WarnContext(ctx, "datahub_legacy_namespace_alias_enabled",
		"legacy_prefix", datahubapi.LegacyNamespacePrefix,
		"removal_condition", "every deployed DataHubService consumer publishes pacts on services.datahub.v1",
	)
	// The allowlist is re-checked per request, not only at handshake time: a
	// TLS terminator moving in front of this process would otherwise silently
	// retire the handshake-time check. See middleware.RequirePeerIdentity.
	handler = middleware.RequirePeerIdentity(dcfg.AllowedPeers, handler)
	handler = middleware.PeerIdentityHTTPMiddleware(handler)

	srv := tlsutil.NewMTLSHTTPServer(dcfg.ListenAddr, tlsCfg, handler)

	// The container probe and Prometheus get the plaintext ops listener rather
	// than a client certificate: handing either one a certificate would put a
	// monitoring identity in DATAHUB_ALLOWED_PEERS, which is the data plane's
	// entire authorisation decision. The listener carries /health and /metrics
	// and no data-plane surface, so it is not a second entrance to anything —
	// e2e/hurl/alt-data-hub/03-ops-listener.hurl asserts the 404s that say so.
	bootstrap.LogOpsWiring(ctx, log, opsAddr, rt.MetricsHandler != nil)
	opsSrv := bootstrap.NewOpsServer(opsAddr, bootstrap.NewOpsHandler(serviceName, rt.MetricsHandler), cfg)

	sup := bootstrap.NewSupervisor(log)
	sup.AddServer("mtls", func() error { return srv.ListenAndServeTLS("", "") }, srv.Shutdown)
	sup.AddServer("ops", opsSrv.ListenAndServe, opsSrv.Shutdown)
	sup.Start(ctx)

	outcome := sup.Wait(ctx)
	bootstrap.LogMemStats(ctx, log)
	cancel()
	sup.GracefulShutdown(ctx, cfg.Server.WriteTimeout)

	log.Info("datahub stopped", "reason", outcome.Reason, "signal", outcome.Signal, "server", outcome.Server)
}

// runHealthcheck self-probes the ops listener and additionally dials the mTLS
// listener.
//
// The second check is the point. A probe that only GETs the plaintext port
// cannot tell a healthy process from one whose mTLS listener goroutine has
// died — the data plane would be unreachable while the container reported
// healthy (ADR-000784). It dials and closes without handshaking: completing a
// handshake would mean adding alt-data-hub to its own DATAHUB_ALLOWED_PEERS,
// muddying what that list means.
func runHealthcheck() {
	opsAddr, err := config.LoadOpsListenAddr()
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		os.Exit(1)
	}
	mtlsAddr, err := config.LoadDataHubListenAddr()
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		os.Exit(1)
	}
	if err := bootstrap.Healthcheck(context.Background(), bootstrap.HealthcheckOptions{
		OpsAddr:    opsAddr,
		TCPTargets: []string{mtlsAddr},
	}); err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
