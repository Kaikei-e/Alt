package infrastructure

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"pki-agent/internal/domain"
)

// PromObserver implements domain.Observer and publishes a small, named
// metric family per subject. One sidecar per service, so the cardinality
// is bounded by the service count (8 today).
type PromObserver struct {
	subject string

	notAfter       prometheus.Gauge
	remaining      prometheus.Gauge
	lastRotation   prometheus.Gauge
	renewalTotal   *prometheus.CounterVec
	reissueTotal   *prometheus.CounterVec
	up             prometheus.Gauge
	healthy        prometheus.Gauge

	mu     sync.RWMutex
	state  domain.CertState
	lastOK time.Time
}

// NewPromObserver registers Prometheus collectors on the default registerer.
// Exactly one observer should be constructed per process.
func NewPromObserver(subject string) *PromObserver {
	labels := prometheus.Labels{"subject": subject}
	o := &PromObserver{
		subject: subject,
		notAfter: promauto.With(prometheus.DefaultRegisterer).NewGauge(prometheus.GaugeOpts{
			Namespace: "pki_agent", Name: "cert_not_after_seconds",
			Help:        "Unix timestamp of the current leaf certificate's not_after.",
			ConstLabels: labels,
		}),
		remaining: promauto.With(prometheus.DefaultRegisterer).NewGauge(prometheus.GaugeOpts{
			Namespace: "pki_agent", Name: "cert_remaining_seconds",
			Help:        "Seconds until the current leaf expires. Negative if expired.",
			ConstLabels: labels,
		}),
		lastRotation: promauto.With(prometheus.DefaultRegisterer).NewGauge(prometheus.GaugeOpts{
			Namespace: "pki_agent", Name: "last_rotation_timestamp_seconds",
			Help:        "Unix timestamp of the last successful cert rotation.",
			ConstLabels: labels,
		}),
		renewalTotal: promauto.With(prometheus.DefaultRegisterer).NewCounterVec(prometheus.CounterOpts{
			Namespace: "pki_agent", Name: "renewal_total",
			Help:        "Count of completed rotation attempts grouped by outcome.",
			ConstLabels: labels,
		}, []string{"result"}),
		reissueTotal: promauto.With(prometheus.DefaultRegisterer).NewCounterVec(prometheus.CounterOpts{
			Namespace: "pki_agent", Name: "reissue_total",
			Help:        "Count of reissuances by reason (missing / expired / near_expiry / corrupt).",
			ConstLabels: labels,
		}, []string{"reason"}),
		up: promauto.With(prometheus.DefaultRegisterer).NewGauge(prometheus.GaugeOpts{
			Namespace: "pki_agent", Name: "up",
			Help:        "1 if the agent process is running.",
			ConstLabels: labels,
		}),
		healthy: promauto.With(prometheus.DefaultRegisterer).NewGauge(prometheus.GaugeOpts{
			Namespace: "pki_agent", Name: "healthy",
			Help:        "1 if the cert on disk is currently valid (not expired).",
			ConstLabels: labels,
		}),
	}
	o.up.Set(1)
	return o
}

func (o *PromObserver) OnClassified(state domain.CertState, remaining time.Duration) {
	o.mu.Lock()
	o.state = state
	if state == domain.StateFresh || state == domain.StateNearExpiry {
		o.lastOK = time.Now()
	}
	o.mu.Unlock()
	o.remaining.Set(remaining.Seconds())
	o.notAfter.Set(float64(time.Now().Add(remaining).Unix()))
	if state == domain.StateExpired || state == domain.StateCorrupt || state == domain.StateMissing {
		o.healthy.Set(0)
	} else {
		o.healthy.Set(1)
	}
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

// State returns the last classified state (used by the /healthz handler).
func (o *PromObserver) State() domain.CertState {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.state
}
