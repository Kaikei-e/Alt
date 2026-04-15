// Package handler exposes /healthz and /metrics over plain HTTP on the
// agent's observation port. These endpoints are scraped from within
// alt-network and are not published outside Docker.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"pki-agent/internal/domain"
)

// StateReader returns the agent's last-known cert state. Implemented by
// infrastructure.PromObserver.
type StateReader interface {
	State() domain.CertState
}

// NewMux builds the observability mux. One endpoint per concern:
//   - GET /healthz  -> 200 if cert is fresh/near_expiry, 503 otherwise
//   - GET /metrics  -> Prometheus exposition
func NewMux(reader StateReader) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		s := reader.State()
		body := map[string]string{"state": s.String()}
		if s == domain.StateFresh || s == domain.StateNearExpiry {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(body)
	})
	return mux
}
