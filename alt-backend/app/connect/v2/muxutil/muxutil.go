// Package muxutil holds the two pieces of Connect-RPC server plumbing that
// every alt-backend binary needs: a health endpoint and the h2c wrapper.
//
// It exists so that the user/operator surfaces (alt/connect/v2) and the
// service-to-service surface (alt/connect/v2/datahub) can share plumbing
// without sharing a package. If they shared one, every binary that imported
// either would link both, and DataHubService's handler would be compiled into
// cmd/backend even though nothing there can mount it.
package muxutil

import (
	"net/http"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// RegisterHealth mounts the /health endpoint every Connect-RPC listener serves.
func RegisterHealth(mux *http.ServeMux) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"connect-rpc"}`))
	})
}

// WithH2C enables HTTP/2 without TLS (h2c), which Connect-RPC's gRPC protocol
// requires on the plaintext listeners.
//
// The single call site keeps the deprecation in one place: the documented
// replacement is http.Server.Protocols, which every listener would have to opt
// into together with the mTLS listener's ALPN config, so the migration is
// deliberately not spread across these constructors.
func WithH2C(mux *http.ServeMux) http.Handler {
	return h2c.NewHandler(mux, &http2.Server{}) //nolint:staticcheck // SA1019: migrating to http.Server.Protocols requires changing every listener at once
}
