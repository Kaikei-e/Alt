package pki

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Handle is the running enrollment loop, stopped on process shutdown.
type Handle struct {
	stop     context.CancelFunc
	done     <-chan struct{}
	closer   func()
	once     sync.Once
	gatherer prometheus.Gatherer
}

// Stop cancels the renewal loop, waits for Run (including in-flight Tick) to
// return, then closes idle HTTP connections. Idempotent. After Stop returns,
// no further cert writes occur.
func (h *Handle) Stop() {
	if h == nil {
		return
	}
	h.once.Do(func() {
		if h.stop != nil {
			h.stop()
		}
		if h.done != nil {
			<-h.done
		}
		if h.closer != nil {
			h.closer()
		}
	})
}

// MetricsHandler serves the dedicated private PKI registry. Nil when
// enrollment is disabled (composition root still opens :9110 and returns 503).
func (h *Handle) MetricsHandler() http.Handler {
	if h == nil || h.gatherer == nil {
		return nil
	}
	return promhttp.HandlerFor(h.gatherer, promhttp.HandlerOpts{})
}

// Gatherer is the dedicated PKI registry, or nil when enrollment is disabled.
func (h *Handle) Gatherer() prometheus.Gatherer {
	if h == nil {
		return nil
	}
	return h.gatherer
}

// Start loads config for serviceName, logs enabled/disabled, and either
// returns a no-op (disabled) or fail-fast enrolls then runs the loop.
func Start(ctx context.Context, log *slog.Logger, serviceName string) (*Handle, error) {
	cfg, err := LoadConfig(serviceName)
	if err != nil {
		return nil, err
	}
	if cfg.Mode != ModeEnabled {
		return StartWith(ctx, log, cfg, nil)
	}
	return startEnabled(ctx, log, cfg, nil)
}

// startEnabled mints onto a private Prometheus registry. PKI series must never
// land on prometheus.DefaultRegisterer / the application mux (F-001).
func startEnabled(ctx context.Context, log *slog.Logger, cfg *Config, issuer Issuer) (*Handle, error) {
	reg := prometheus.NewRegistry()
	h, err := StartWithObserver(ctx, log, cfg, issuer, NewPromObserver(cfg.Subject, reg))
	if err != nil {
		return nil, err
	}
	if h != nil {
		h.gatherer = reg
	}
	return h, nil
}

// StartWith is the test seam: a non-nil issuer skips the native CA client so
// unit tests never talk to a live CA.
func StartWith(ctx context.Context, log *slog.Logger, cfg *Config, issuer Issuer) (*Handle, error) {
	return StartWithObserver(ctx, log, cfg, issuer, nopObserver{})
}

// StartWithObserver is the test seam that injects a metrics Observer.
func StartWithObserver(ctx context.Context, log *slog.Logger, cfg *Config, issuer Issuer, obs Observer) (*Handle, error) {
	if log == nil {
		log = slog.Default()
	}
	if cfg.Mode != ModeEnabled {
		log.InfoContext(ctx, "pki_enrollment_disabled",
			"service", cfg.Subject, "mode", cfg.Mode,
			"reason", "migration compatibility: sidecar still owns cert files until compose cutover sets PKI_ENROLLMENT=enabled")
		return nil, nil
	}
	log.InfoContext(ctx, "pki_enrollment_enabled",
		"service", cfg.Subject,
		"provisioner", cfg.Provisioner,
		"password_file", cfg.PasswordFile,
		"cert_path", cfg.CertPath,
	)
	if issuer == nil {
		issuer = &NativeStepCAIssuer{
			CAURL:        cfg.CAURL,
			RootFile:     cfg.RootFile,
			Provisioner:  cfg.Provisioner,
			PasswordFile: cfg.PasswordFile,
		}
	}
	mgr := &Manager{
		Cfg:      *cfg,
		Issuer:   issuer,
		Files:    &CertFile{CertPath: cfg.CertPath, KeyPath: cfg.KeyPath},
		Log:      log,
		Observer: obs,
	}
	if err := mgr.Enroll(ctx); err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithCancel(ctx) // #nosec G118 -- Handle.Stop invokes cancel
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cancel()
		_ = mgr.Run(runCtx)
	}()
	closer := func() {}
	if c, ok := issuer.(interface{ CloseIdleConnections() }); ok {
		closer = c.CloseIdleConnections
	}
	return &Handle{stop: cancel, done: done, closer: closer}, nil
}
