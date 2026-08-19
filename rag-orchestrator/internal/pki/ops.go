package pki

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	defaultOpsListenAddr = "127.0.0.1:9110"
	opsListenEnv         = "OPS_LISTEN"
)

// LoadOpsListenAddr returns the bind address of the dedicated PKI ops listener.
// Default 127.0.0.1:9110 keeps a laptop `go run` off the LAN; compose can set
// OPS_LISTEN=:9110. This listener is independent of the Echo API /metrics mux.
func LoadOpsListenAddr() (string, error) {
	addr := strings.TrimSpace(os.Getenv(opsListenEnv))
	if addr == "" {
		return defaultOpsListenAddr, nil
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("%s=%q is not a valid host:port: %w", opsListenEnv, addr, err)
	}
	if strings.TrimSpace(port) == "" {
		return "", fmt.Errorf("%s=%q has no port", opsListenEnv, addr)
	}
	return addr, nil
}

// NewOpsHandler serves /health and /metrics from gatherer. The gatherer MUST
// be a private registry — never prometheus.DefaultGatherer — so PKI series
// are not published on the Echo API mux.
func NewOpsHandler(gatherer prometheus.Gatherer) http.Handler {
	if gatherer == nil {
		gatherer = prometheus.NewRegistry()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "healthy",
			"service": "rag-orchestrator",
		})
	})
	mux.Handle("/metrics", promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}))
	return mux
}

// NewOpsServer binds the PKI ops surface with Slowloris-safe timeouts.
func NewOpsServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}
