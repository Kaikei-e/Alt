package middleware

import (
	"log/slog"
	"net/http"
)

// PlaintextHealthOnly is an http.Handler wrapper that rejects all routes except
// /health. This closes A04: business endpoints carrying session cookies and
// backend JWTs must not be served over unencrypted HTTP.
//
// Wrapping the plaintext listener's handler directly prevents this guard
// from breaking the mTLS and frontend HTTPS listeners (which use the same Echo router).
func PlaintextHealthOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		if _, err := w.Write([]byte(`{"error": "business endpoints require HTTPS"}`)); err != nil {
			slog.WarnContext(r.Context(), "failed to write plaintext health guard response", "error", err)
		}
	})
}
