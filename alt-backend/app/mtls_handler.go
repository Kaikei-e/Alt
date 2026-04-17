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

// buildMTLSHandler composes a single http.Handler that routes Connect-RPC
// traffic to connectMux and all other traffic (notably REST routes such as
// /v1/recap/articles) to echoHandler. The mTLS listener uses this so that
// both transports coexist on :9443 after the mTLS cutover.
func buildMTLSHandler(connectMux http.Handler, echoHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, prefix := range connectRPCPrefixes {
			if strings.HasPrefix(r.URL.Path, prefix) {
				connectMux.ServeHTTP(w, r)
				return
			}
		}
		echoHandler.ServeHTTP(w, r)
	})
}
