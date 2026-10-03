package pki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// NewOpsHandler builds the dedicated PKI ops surface: /health and /metrics.
// The mux is explicit and never http.DefaultServeMux. A nil metrics handler
// returns 503 on /metrics so scrape is distinguishable from "no such route".
func NewOpsHandler(serviceName string, metrics http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "healthy",
			"service": serviceName,
		})
	})
	if metrics != nil {
		mux.Handle("/metrics", metrics)
	} else {
		mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("metrics exporter is not wired: PKI enrollment is disabled or has no private registry\n"))
		})
	}
	return mux
}

// NewOpsServer builds the ops listener with the shared GREEN timeouts.
func NewOpsServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// OpsListenAddr is 127.0.0.1:9110 unless OPS_LISTEN is set (compose later
// uses :9110 so Prometheus can scrape the parent netns).
func OpsListenAddr() string {
	if v := strings.TrimSpace(os.Getenv(opsListenEnv)); v != "" {
		return v
	}
	return defaultOpsListenAddr
}

// ListenOps binds the dedicated ops listener and serves it in a goroutine.
func ListenOps(ctx context.Context, log *slog.Logger, serviceName string, metrics http.Handler) (*http.Server, error) {
	if log == nil {
		log = slog.Default()
	}
	addr := OpsListenAddr()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("pki: ops listen %s: %w", addr, err)
	}
	srv := NewOpsServer(ln.Addr().String(), NewOpsHandler(serviceName, metrics))
	log.InfoContext(ctx, "ops_listener.wiring",
		"addr", srv.Addr,
		"reach", listenReach(addr),
		"surfaces", "/health,/metrics",
		"auth", "none",
		"metrics_enabled", metrics != nil,
	)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.ErrorContext(ctx, "ops_listener.serve", "error", err)
		}
	}()
	return srv, nil
}

// ShutdownOps stops the ops listener. Nil-safe and idempotent.
func ShutdownOps(ctx context.Context, srv *http.Server) error {
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

func listenReach(addr string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return "container_network"
	}
	if host == "" {
		return "container_network"
	}
	if host == "localhost" {
		return "loopback_only"
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return "loopback_only"
	}
	return "container_network"
}
