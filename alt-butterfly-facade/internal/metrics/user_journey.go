// Package metrics records user-journey SLO counters at the BFF edge.
//
// Wave 3 (P2-12) pages on burn rate of
// alt_user_journey_requests_total{journey,status}. The BFF is the hop closest
// to user impact: it sees auth 401s, circuit-breaker 503s and upstream 5xx
// that alt-backend's own handlers never observe.
//
// Login is session-token validation, not every API call and not the Kratos
// password form. The BFF has no whoami route today; login samples are
// session-shaped paths plus 401s (and 5xx on those paths). Product journeys
// still record their own errors on 401 so feeds/search outages are not hidden.
package metrics

import (
	"net/http"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricName       = "alt_user_journey_requests_total"
	instrumentedName = "alt_user_journey_instrumented"

	JourneyFeeds  = "feeds"
	JourneyLogin  = "login"
	JourneySearch = "search"

	StatusOK    = "ok"
	StatusError = "error"
)

// Metrics holds the journey counter. Tests inject a registry; production
// uses Default() so /metrics on this process exports it once per process.
type Metrics struct {
	requests     *prometheus.CounterVec
	instrumented prometheus.Gauge
}

var (
	defaultOnce    sync.Once
	defaultMetrics *Metrics
)

// Default is the process-wide counter registered on prometheus.DefaultRegisterer.
func Default() *Metrics {
	defaultOnce.Do(func() {
		defaultMetrics = New(prometheus.DefaultRegisterer)
	})
	return defaultMetrics
}

// New registers alt_user_journey_requests_total and the instrumented gauge.
// Request series are created only by Record: pre-created zeros would make
// absent() and increase==0 look like coverage.
func New(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricName,
		Help: "User-journey requests at the BFF edge, labeled by journey and outcome.",
	}, []string{"journey", "status"})
	instrumented := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: instrumentedName,
		Help: "1 when the BFF user-journey SLO counter is registered in this process.",
	})
	reg.MustRegister(requests, instrumented)
	instrumented.Set(1)

	return &Metrics{requests: requests, instrumented: instrumented}
}

// JourneyForPath maps a BFF request path onto a user-journey SLI, or "" if
// the path is not feeds, search, or a session-validation event.
func JourneyForPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/alt.feeds.v2.FeedService/"),
		strings.HasPrefix(path, "/v1/feeds"):
		return JourneyFeeds
	case strings.HasPrefix(path, "/alt.search.v2.SearchService/"),
		strings.HasPrefix(path, "/alt.search.v2.GlobalSearchService/"):
		return JourneySearch
	case isSessionPath(path):
		return JourneyLogin
	default:
		return ""
	}
}

func isSessionPath(path string) bool {
	return strings.Contains(path, "/sessions/") ||
		strings.HasSuffix(path, "/whoami") ||
		strings.HasPrefix(path, "/ory/")
}

func statusLabel(code int) string {
	if code == http.StatusUnauthorized ||
		code == http.StatusTooManyRequests ||
		code >= http.StatusInternalServerError {
		return StatusError
	}
	return StatusOK
}

// Record increments the SLO counter for this completed request.
//
// Login is only session-validation events (session-shaped paths, or 401).
// 401 on feeds/search also increments that journey as error so product
// outages are not hidden behind a login-only page. 5xx/429 on a classified
// path are that journey's error and are not login=ok.
func (m *Metrics) Record(path string, code int) {
	if m == nil || m.requests == nil {
		return
	}
	journey := JourneyForPath(path)
	if journey == JourneyLogin {
		m.requests.WithLabelValues(JourneyLogin, statusLabel(code)).Inc()
		return
	}
	if code == http.StatusUnauthorized {
		m.requests.WithLabelValues(JourneyLogin, StatusError).Inc()
		if journey != "" {
			m.requests.WithLabelValues(journey, StatusError).Inc()
		}
		return
	}
	if journey != "" {
		m.requests.WithLabelValues(journey, statusLabel(code)).Inc()
	}
}
