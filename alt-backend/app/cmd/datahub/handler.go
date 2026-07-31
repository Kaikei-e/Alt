package main

import (
	"net/http"
	"strings"
)

// connectRPCPrefix is the URL path prefix served by the Connect-RPC mux.
//
// The one namespace this binary answers on, alt.datahub.v1.DataHubService,
// falls under it. Routing is prefix-based rather than a list of procedures, so
// adding a capability to the proto does not need a matching edit in this file
// — the connect mux is what decides whether a procedure exists, and it 404s
// the ones that do not.
//
// "/services." was here until ADR-000954 Wave 2-C, for
// services.backend.v1.BackendInternalService. It is gone rather than kept as a
// harmless leftover: a prefix in this list is a promise that something behind
// it may answer, and a caller on the retired path should reach the same 404 as
// a caller on a path that never existed.
const connectRPCPrefix = "/alt."

func isConnectRPCPath(path string) bool {
	return strings.HasPrefix(path, connectRPCPrefix)
}

// dataHubHandler composes the single handler behind data-hub's mutual-TLS
// listener: Connect-RPC traffic and /health to connectMux, and a 404 for
// everything else.
//
// The 404 is the point. Its predecessor (buildMTLSHandler) fell through to the
// full browser-facing Echo router, so one socket answered for the user API,
// the admin API and the service-to-service API at once — and whether it
// verified the caller's certificate depended on an environment variable that
// defaulted to "no". Here there is nothing to fall through to: the user
// surfaces are not compiled into this binary, and the /v1/internal REST group
// that shared this listener through Wave 2 was folded into DataHubService by
// ADR-000954 D6 and deleted.
func dataHubHandler(connectMux http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isConnectRPCPath(r.URL.Path), r.URL.Path == "/health":
			connectMux.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}
