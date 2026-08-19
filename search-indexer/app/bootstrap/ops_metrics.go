package bootstrap

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"search-indexer/healthdeep"
)

// opsMetricsHandler scrapes PKI enrollment (when present) plus deep-health
// gauges on the dedicated :9110 registry. health_deep_* must not land on
// prometheus.DefaultRegisterer.
func opsMetricsHandler(pki prometheus.Gatherer) http.Handler {
	var gs prometheus.Gatherers
	if pki != nil {
		gs = append(gs, pki)
	}
	gs = append(gs, healthdeep.Gatherer())
	return promhttp.HandlerFor(gs, promhttp.HandlerOpts{})
}
