package middleware

import (
	"log/slog"
	"net/http"

	"alt/utils/logger"
)

// PeerIdentityHTTPMiddleware wraps h so that requests arriving over TLS have
// their client-cert CommonName captured and logged. Requests on plaintext
// listeners pass through unchanged. This closes the T3 audit-log gap where
// the previous shared-token scheme could not distinguish which service made
// a given internal call.
func PeerIdentityHTTPMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			peer := r.TLS.PeerCertificates[0].Subject.CommonName
			ctx := r.Context()
			logger.Logger.LogAttrs(ctx, slog.LevelInfo, "mtls peer",
				slog.String("peer", peer),
				slog.String("path", r.URL.Path),
			)
			r = r.WithContext(ctx)
		}
		h.ServeHTTP(w, r)
	})
}
