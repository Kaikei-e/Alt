// Package muxutil holds the Connect-RPC server plumbing that every
// alt-backend binary needs: the /health endpoint every Connect listener
// serves.
//
// It exists so that the user/operator surfaces (alt/connect/v2) and the
// service-to-service surface (alt/connect/v2/datahub) can share plumbing
// without sharing a package. If they shared one, every binary that imported
// either would link both, and DataHubService's handler would be compiled into
// cmd/backend even though nothing there can mount it.
//
// Cleartext HTTP/2 (h2c) used to be enabled here by wrapping the mux in
// golang.org/x/net/http2/h2c. That package is deprecated: since Go 1.24 the
// standard library carries the same capability as http.Server.Protocols, which
// is a *server* field and therefore cannot be expressed by a function that
// returns an http.Handler. The switch now lives with the listeners that need
// it — internal/bootstrap.NewConnectServer and NewServiceServer — so it is set
// exactly where the http.Server is built.
package muxutil

import "net/http"

// RegisterHealth mounts the /health endpoint every Connect-RPC listener serves.
func RegisterHealth(mux *http.ServeMux) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"connect-rpc"}`))
	})
}
