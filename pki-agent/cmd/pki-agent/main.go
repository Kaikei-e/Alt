// Command pki-agent is a single-responsibility sidecar that keeps one mTLS
// leaf certificate on the configured volume within its validity window.
// One process per consumer service (alt-backend, auth-hub, …); the process
// is not internally concurrent beyond a single rotation ticker.
//
// Lifecycle:
//   - Tick on startup (issues cert if missing/expired).
//   - Tick on TICK_INTERVAL (default 5m). At ~66% of cert lifetime, reissue.
//   - SIGTERM/SIGINT drain the ticker and shut down the metrics server.
//
// Replaces the compose-embedded shell cert-init + cert-renewer pair.
package main

import (
	"context"
	"errors"
	"flag"
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
		healthcheck()
		return
	}

	flag.Parse()
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

	// Metrics + health server.
	srv := &http.Server{
		Addr:              cfg.MetricsAddr,
		Handler:           handler.NewMux(obs),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		slog.Info("metrics server listening", "addr", cfg.MetricsAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("metrics server", "err", err)
		}
	}()

	// Ticker loop with jitter-free fixed interval. Rotation is idempotent
	// per state, so we don't need to randomize.
	ticker := time.NewTicker(cfg.TickInterval)
	defer ticker.Stop()
	consecutiveFailures := 0
	for {
		select {
		case <-ctx.Done():
			slog.Info("shutting down")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = srv.Shutdown(shutdownCtx)
			cancel()
			return
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
		}
	}
}

// healthcheck is invoked as the Docker HEALTHCHECK command. It talks to the
// in-process /healthz to get the agent's view. Plain HTTP to loopback.
func healthcheck() {
	addr := os.Getenv("METRICS_ADDR")
	if addr == "" {
		addr = ":9510"
	}
	if addr[0] == ':' {
		addr = "127.0.0.1" + addr
	}
	url := "http://" + addr + "/healthz"
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		os.Exit(1)
	}
}

// silence unused import warnings when domain isn't directly referenced.
var _ = domain.StateFresh
