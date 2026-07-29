package main

import (
	"net/http"
	"strings"
)

// connectRPCPrefixes are the URL path prefixes served by the Connect-RPC mux.
// Requests matching any of these go through the connect mux; everything else
// falls through to the echo REST handler.
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

// buildMTLSHandler composes a single http.Handler for the mTLS listener:
// Connect-RPC traffic goes to connectMux, /v1/internal/* to internalREST, and
// everything else (notably REST routes such as /v1/recap/articles) to
// echoHandler. Both transports coexist on :9443 after the mTLS cutover.
func buildMTLSHandler(connectMux http.Handler, echoHandler http.Handler, internalREST http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isConnectRPCPath(r.URL.Path):
			connectMux.ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, internalRESTPrefix):
			internalREST.ServeHTTP(w, r)
		default:
			echoHandler.ServeHTTP(w, r)
		}
	})
}

// buildInternalHandler composes the handler for the internal listener. That
// port is bound to loopback on the host, and it carries only the
// service-to-service surfaces: the internal Connect-RPC mux and
// /v1/internal/*. Anything else is a 404 so the listener cannot double as a
// second entrance to the user-facing API.
func buildInternalHandler(internalConnectMux http.Handler, internalREST http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isConnectRPCPath(r.URL.Path):
			internalConnectMux.ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, internalRESTPrefix), r.URL.Path == "/health":
			internalREST.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}
