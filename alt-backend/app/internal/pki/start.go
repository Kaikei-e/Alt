package pki

import (
	"context"
	"log/slog"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// Handle is the running enrollment loop, stopped on process shutdown.
type Handle struct {
	stop   context.CancelFunc
	done   <-chan struct{}
	closer func()
	once   sync.Once
}

// Stop cancels the renewal loop and waits for Run to return, including any
// in-flight Tick. Idle HTTP connections are closed afterwards. Idempotent.
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

// Start loads config for serviceName, logs enabled/disabled, and either
// returns a no-op (disabled) or fail-fast enrolls then runs the loop.
// Metrics register on prometheus.DefaultRegisterer so the first-cohort ops
// listener (:9110) keeps scraping them. Services that must not share the
// default registry should call StartWithRegisterer.
func Start(ctx context.Context, log *slog.Logger, serviceName string) (*Handle, error) {
	return StartWithRegisterer(ctx, log, serviceName, nil)
}

// StartWithRegisterer is Start with an explicit Prometheus registerer.
// A nil registerer uses DefaultRegisterer (first-cohort ops :9110).
func StartWithRegisterer(ctx context.Context, log *slog.Logger, serviceName string, reg prometheus.Registerer) (*Handle, error) {
	cfg, err := LoadConfig(serviceName)
	if err != nil {
		return nil, err
	}
	if cfg.Mode != ModeEnabled {
		return StartWith(ctx, log, cfg, nil)
	}
	return StartWithObserver(ctx, log, cfg, nil, NewPromObserver(cfg.Subject, reg))
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
			"reason", "sidecar still owns cert files until compose cutover for remaining subjects")
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
	runCtx, cancel := context.WithCancel(ctx)
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
