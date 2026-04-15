package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"search-indexer/logger"
)

// PeerIdentityHeader is the header downstream handlers use to read the
// authenticated caller's identity (TLS client-cert CommonName).
const PeerIdentityHeader = "X-Alt-Peer-Identity"

// PeerIdentityMiddleware validates that a request arrived over mTLS from a
// caller in the allowlist. The callers are matched by TLS client cert CN.
//
// On mismatch the handler returns 403 Forbidden and the raw CN is written
// to the structured log so auditors can see the offending peer. On match
// the CN is propagated downstream via the X-Alt-Peer-Identity header so
// handler code can log / partition / authorize on the authenticated caller.
//
// Plaintext requests (r.TLS == nil) are refused with 401 — this middleware
// is intended to be wired only on the :9443 mTLS listener, never on the
// plaintext listener. Fail-closed if mTLS context is missing is the
// right choice for internal-only endpoints.
type PeerIdentityMiddleware struct {
	allowedCallers map[string]struct{}
}

// NewPeerIdentityMiddleware returns a middleware that only permits the given
// CNs. Empty allowlist == fail-closed (all requests rejected).
func NewPeerIdentityMiddleware(allowed []string) *PeerIdentityMiddleware {
	m := &PeerIdentityMiddleware{allowedCallers: make(map[string]struct{}, len(allowed))}
	for _, cn := range allowed {
		cn = strings.TrimSpace(cn)
		if cn == "" {
			continue
		}
		m.allowedCallers[cn] = struct{}{}
	}
	return m
}

// Require wraps next so that only connections with an allowed client cert CN
// reach it.
func (m *PeerIdentityMiddleware) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			logger.Logger.LogAttrs(r.Context(), slog.LevelWarn,
				"peer_identity: missing mTLS client cert",
				slog.String("path", r.URL.Path),
			)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		cn := r.TLS.PeerCertificates[0].Subject.CommonName
		if _, ok := m.allowedCallers[cn]; !ok {
			logger.Logger.LogAttrs(r.Context(), slog.LevelWarn,
				"peer_identity: caller not in allowlist",
				slog.String("peer", cn),
				slog.String("path", r.URL.Path),
			)
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		// Normalize header: strip any client-sent value, set our own.
		r.Header.Set(PeerIdentityHeader, cn)
		logger.Logger.LogAttrs(r.Context(), slog.LevelDebug,
			"peer_identity: verified",
			slog.String("peer", cn),
			slog.String("path", r.URL.Path),
		)
		next.ServeHTTP(w, r)
	})
}
