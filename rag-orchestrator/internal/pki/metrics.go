package pki

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// PromObserver implements Observer and publishes the in-process enrollment
// metric family on a caller-supplied registry. rag-orchestrator scrapes PKI
// series from the dedicated ops listener (:9110), never from Echo /metrics.
type PromObserver struct {
	subject string

	notAfter     prometheus.Gauge
	remaining    prometheus.Gauge
	lastRotation prometheus.Gauge
	renewalTotal *prometheus.CounterVec
	reissueTotal *prometheus.CounterVec
	healthy      prometheus.Gauge

	mu    sync.RWMutex
	state State
}

// NewPromObserver registers collectors on reg (DefaultRegisterer when nil).
// Tests pass a private registry so they never collide with production.
func NewPromObserver(subject string, reg prometheus.Registerer) *PromObserver {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	factory := promauto.With(reg)
	labels := prometheus.Labels{"subject": subject}
	return &PromObserver{
		subject: subject,
		notAfter: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: "pki_enrollment", Name: "cert_not_after_seconds",
			Help:        "Unix timestamp of the current leaf certificate's not_after.",
			ConstLabels: labels,
		}),
		remaining: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: "pki_enrollment", Name: "cert_remaining_seconds",
			Help:        "Seconds until the current leaf expires. Negative if expired.",
			ConstLabels: labels,
		}),
		lastRotation: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: "pki_enrollment", Name: "last_rotation_timestamp_seconds",
			Help:        "Unix timestamp of the last successful cert rotation.",
			ConstLabels: labels,
		}),
		renewalTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pki_enrollment", Name: "renewal_total",
			Help:        "Count of completed rotation attempts grouped by outcome.",
			ConstLabels: labels,
		}, []string{"result"}),
		reissueTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pki_enrollment", Name: "reissue_total",
			Help:        "Count of reissuances by reason (missing / expired / near_expiry / corrupt).",
			ConstLabels: labels,
		}, []string{"reason"}),
		healthy: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: "pki_enrollment", Name: "healthy",
			Help:        "1 if the cert on disk is currently valid (not expired).",
			ConstLabels: labels,
		}),
	}
}

func (o *PromObserver) OnClassified(state State, remaining time.Duration) {
	o.mu.Lock()
	o.state = state
	o.mu.Unlock()
	o.remaining.Set(remaining.Seconds())
	o.notAfter.Set(float64(time.Now().Add(remaining).Unix()))
	if state == StateExpired || state == StateCorrupt || state == StateMissing {
		o.healthy.Set(0)
		return
	}
	o.healthy.Set(1)
}

func (o *PromObserver) OnReissued(reason string) {
	o.reissueTotal.WithLabelValues(reason).Inc()
}

func (o *PromObserver) OnRenewed(success bool) {
	result := "success"
	if !success {
		result = "failure"
	} else {
		o.lastRotation.Set(float64(time.Now().Unix()))
	}
	o.renewalTotal.WithLabelValues(result).Inc()
}

func (o *PromObserver) OnRetry(int, error) {}
