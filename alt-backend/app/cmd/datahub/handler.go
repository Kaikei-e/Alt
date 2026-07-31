package main

import (
	"net/http"
	"strings"
)

// connectRPCPrefixes are the URL path prefixes served by the Connect-RPC mux.
//
// Both proto namespaces this binary answers on fall under these two prefixes:
// alt.datahub.v1.DataHubService under "/alt.", and the legacy
// services.backend.v1.BackendInternalService under "/services." until
// ADR-000954 Wave 2-C retires it. Routing here is prefix-based rather than a
// list of procedures, so adding a capability to the proto does not need a
// matching edit in this file — the connect mux is what decides whether a
// procedure exists, and it 404s the ones that do not.
var connectRPCPrefixes = []string{
	"/alt.",
	"/services.",
}

// internalRESTPrefix is the REST namespace whose handlers take no tenant
// argument and carry no auth middleware.
const internalRESTPrefix = "/v1/internal"

func isConnectRPCPath(path string) bool {
	for _, prefix := range connectRPCPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// dataHubHandler composes the single handler behind data-hub's mutual-TLS
// listener: Connect-RPC traffic to connectMux, /v1/internal/* and /health to
// internalREST, and a 404 for everything else.
//
// The 404 is the point. Its predecessor (buildMTLSHandler) fell through to the
// full browser-facing Echo router, so one socket answered for the user API,
// the admin API and the service-to-service API at once — and whether it
// verified the caller's certificate depended on an environment variable that
// defaulted to "no". Here there is nothing to fall through to: the user
// surfaces are not compiled into this binary.
func dataHubHandler(connectMux http.Handler, internalREST http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isConnectRPCPath(r.URL.Path):
			connectMux.ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, internalRESTPrefix), r.URL.Path == "/health":
			internalREST.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}
