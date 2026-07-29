package middleware

import (
	"log/slog"
	"net/http"

	"alt/utils/logger"
)

// PeerIdentityHeader carries the authenticated caller's identity to handlers
// and to the audit log. Only a verified client certificate may set it.
const PeerIdentityHeader = "X-Alt-Peer-Identity"

// PeerIdentityHTTPMiddleware wraps h so that requests arriving over TLS have
// their client-cert CommonName captured and logged. This closes the T3
// audit-log gap where the previous shared-token scheme could not distinguish
// which service made a given internal call.
//
// The header is deleted before anything else runs: this handler is also
// mounted on plaintext listeners, where nothing verifies a certificate, so a
// caller-supplied value would otherwise reach the handlers as if the
// transport had vouched for it (pki-agent's proxy does the same).
func PeerIdentityHTTPMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del(PeerIdentityHeader)
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			peer := r.TLS.PeerCertificates[0].Subject.CommonName
			r.Header.Set(PeerIdentityHeader, peer)
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
