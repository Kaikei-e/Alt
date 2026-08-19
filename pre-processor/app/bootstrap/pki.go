package bootstrap

import (
	"context"
	"log/slog"
	"net/http"

	"pre-processor/internal/pki"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const pkiServiceName = "pre-processor"

// pkiEnrollment is the composition-root result of in-process cert lifecycle.
// gatherer is nil when enrollment is disabled (ops :9110 still binds, /metrics 503).
type pkiEnrollment struct {
	handle   *pki.Handle
	gatherer prometheus.Gatherer
}

func (e *pkiEnrollment) stop() {
	if e == nil {
		return
	}
	e.handle.Stop()
}

// startEnrollment fail-fast enrolls when PKI_ENROLLMENT=enabled, then returns
// the renewal loop handle. It must run before any TLS client or listener
// loads cert files. Metrics go on a private registry served at parent :9110;
// they must not share the :9201 collector (that would double-count scrapes).
func startEnrollment(ctx context.Context, log *slog.Logger) (*pkiEnrollment, error) {
	cfg, err := pki.LoadConfig(pkiServiceName)
	if err != nil {
		return nil, err
	}
	if cfg.Mode != pki.ModeEnabled {
		h, err := pki.StartWith(ctx, log, cfg, nil)
		if err != nil {
			return nil, err
		}
		return &pkiEnrollment{handle: h}, nil
	}
	reg := prometheus.NewRegistry()
	h, err := pki.StartWithObserver(ctx, log, cfg, nil, pki.NewPromObserver(cfg.Subject, reg))
	if err != nil {
		return nil, err
	}
	return &pkiEnrollment{handle: h, gatherer: reg}, nil
}

func (e *pkiEnrollment) metricsHandler() http.Handler {
	if e == nil || e.gatherer == nil {
		return nil
	}
	return promhttp.HandlerFor(e.gatherer, promhttp.HandlerOpts{})
}
