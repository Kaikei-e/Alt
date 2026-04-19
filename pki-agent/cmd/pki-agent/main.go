// Command pki-agent is a single-responsibility sidecar that keeps one mTLS
// leaf certificate on the configured volume within its validity window.
// One process per consumer service (alt-backend, auth-hub, …); the process
// is not internally concurrent beyond a single rotation ticker.
//
// Lifecycle:
//   - Tick on startup (issues cert if missing/expired).
//   - Tick on TICK_INTERVAL (default 5m). At ~66% of cert lifetime, reissue.
//   - SIGTERM/SIGINT drain the ticker and shut down the metrics server.
//   - Any long-lived server goroutine (metrics, optional TLS reverse proxy)
//     exiting with a non-ErrServerClosed error is fatal to the whole
//     process. Docker's `restart: unless-stopped` policy then respawns the
//     container so it rejoins its parent service's netns. This avoids the
//     silent-listener-death failure mode where the container stayed marked
//     healthy while :9443 had stopped serving.
//
// Replaces the compose-embedded shell cert-init + cert-renewer pair.
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

	"pki-agent/config"
	"pki-agent/internal/adapter/handler"
	"pki-agent/internal/domain"
	"pki-agent/internal/infrastructure"
	"pki-agent/internal/usecase"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(os.Getenv("METRICS_ADDR"), os.Getenv("PROXY_LISTEN")); err != nil {
			fmt.Fprintln(os.Stderr, "healthcheck:", err)
			os.Exit(1)
		}
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(2)
	}
	slog.Info("pki-agent starting",
		"subject", cfg.Subject, "sans", cfg.SANs,
		"ca_url", cfg.CAURL, "cert_path", cfg.CertPath,
		"renew_fraction", cfg.RenewAtFraction, "tick_interval", cfg.TickInterval)

	obs := infrastructure.NewPromObserver(cfg.Subject)
	certFile := &infrastructure.CertFile{
		CertPath: cfg.CertPath, KeyPath: cfg.KeyPath,
		OwnerUID: cfg.OwnerUID, OwnerGID: cfg.OwnerGID,
	}
	stepCA := &infrastructure.StepCACLI{
		CAURL: cfg.CAURL, RootFile: cfg.RootFile,
		Provisioner: cfg.Provisioner, PasswordFile: cfg.PasswordFile,
	}
	rotator := &usecase.Rotator{
		Subject: cfg.Subject, SANs: cfg.SANs,
		RenewAtFraction: cfg.RenewAtFraction,
		Loader:          certFile,
		Issuer:          stepCA,
		Writer:          certFile,
		Observer:        obs,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// First tick synchronously so we don't expose /healthz green before we
	// know we can reach step-ca. If this fails we keep running so the
	// ticker can retry with backoff, but we log loudly.
	tickCtx, tickCancel := context.WithTimeout(ctx, 30*time.Second)
	state, err := rotator.Tick(tickCtx, time.Now())
	tickCancel()
	if err != nil {
		slog.Error("initial tick failed", "err", err, "state", state.String())
	} else {
		slog.Info("initial tick ok", "state", state.String())
	}

	// serverErr collects the first non-ErrServerClosed exit from any
	// long-lived server goroutine. Buffered so a goroutine exit does not
	// block on an unselected send. We treat any such exit as fatal (see
	// package doc) so Docker restart-unless-stopped respawns the container.
	serverErr := make(chan error, 2)

	srv := &http.Server{
		Addr:              cfg.MetricsAddr,
		Handler:           handler.NewMux(obs),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		slog.Info("metrics server listening", "addr", cfg.MetricsAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- fmt.Errorf("metrics server: %w", err)
			return
		}
	}()

	var proxySrv *http.Server
	if cfg.ProxyListen != "" {
		proxySrv, err = handler.NewTLSProxy(handler.ProxyConfig{
			Listen:       cfg.ProxyListen,
			Upstream:     cfg.ProxyUpstream,
			CertPath:     cfg.CertPath,
			KeyPath:      cfg.KeyPath,
			CAPath:       cfg.ProxyCAPath,
			VerifyClient: cfg.ProxyVerifyClient,
			AllowedPeers: cfg.ProxyAllowedPeers,
		})
		if err != nil {
			slog.Error("proxy setup failed", "err", err)
			os.Exit(3)
		}
		go func() {
			slog.Info("TLS reverse proxy listening", "addr", cfg.ProxyListen, "upstream", cfg.ProxyUpstream)
			if err := proxySrv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				serverErr <- fmt.Errorf("proxy server: %w", err)
				return
			}
		}()
	}

	shutdown := func(code int) {
		shutdownCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		_ = srv.Shutdown(shutdownCtx)
		if proxySrv != nil {
			_ = proxySrv.Shutdown(shutdownCtx)
		}
		os.Exit(code)
	}

	// Ticker loop with jitter-free fixed interval. Rotation is idempotent
	// per state, so we don't need to randomize.
	ticker := time.NewTicker(cfg.TickInterval)
	defer ticker.Stop()
	consecutiveFailures := 0
	var probeExit probeState
	for {
		select {
		case <-ctx.Done():
			slog.Info("shutting down")
			shutdown(0)
		case err := <-serverErr:
			// A server goroutine has exited unexpectedly. Treat as fatal
			// so Docker respawns the container. Catching this here (rather
			// than letting the goroutine die quietly) is the whole point of
			// the refactor: silent-death was the root cause of the
			// AcolyteService/ListReports 502 incident.
			slog.Error("server exited — terminating", "err", err)
			shutdown(4)
		case <-ticker.C:
			tickCtx, tickCancel := context.WithTimeout(ctx, 30*time.Second)
			state, err := rotator.Tick(tickCtx, time.Now())
			tickCancel()
			if err != nil {
				consecutiveFailures++
				slog.Error("tick failed", "err", err, "state", state.String(), "consecutive_failures", consecutiveFailures)
				continue
			}
			consecutiveFailures = 0
			slog.Info("tick ok", "state", state.String())

			// Self-probe: runHealthcheck combines /healthz, netns interface
			// check and TCP dial of the reverse-proxy listener. Running it
			// from the tick loop (not just from Docker HEALTHCHECK) is what
			// closes the netns-orphan failure mode — Docker does not restart
			// an unhealthy container, so the sidecar has to exit itself when
			// the condition is structural. See ADR-000785.
			if cfg.ProxyListen != "" {
				perr := runHealthcheck(cfg.MetricsAddr, cfg.ProxyListen)
				shouldExit := probeExit.evalProbeResult(perr)
				if perr != nil {
					slog.Error("self-probe failed",
						"err", perr,
						"consecutive", probeExit.consecutive,
						"threshold", probeFailureThreshold)
				}
				if shouldExit {
					slog.Error("self-probe threshold exceeded — exiting to let compose restart respawn in parent's current netns",
						"err", perr)
					shutdown(1)
				}
			}
		}
	}
}

// silence unused import warnings when domain isn't directly referenced.
var _ = domain.StateFresh
