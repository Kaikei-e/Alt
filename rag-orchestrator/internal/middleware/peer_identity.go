// Package middleware contains HTTP middleware for rag-orchestrator.
//
// PeerIdentityMiddleware reads the TLS client-cert CommonName from r.TLS and
// enforces an allowlist.
//
// NOT CURRENTLY WIRED: cmd/server/main.go only starts plaintext listeners
// (cfg.Server.Port for REST, cfg.Server.ConnectPort for Connect-RPC h2c);
// there is no mTLS listener, and NewPeerIdentityMiddleware is not called
// anywhere in the composition root. This control is dead code today and
// provides no protection — do not assume peer identity is enforced on any
// rag-orchestrator endpoint until a real mTLS listener is constructed in
// main.go and Require() is added to its handler chain. Applying Require() to
// either existing plaintext listener would reject every request (r.TLS is
// always nil there), so it must only ever be attached to a real TLS listener
// that terminates client certs. Standing that listener up needs net-new
// server-side TLS plumbing (internal/infra/tlsutil currently only builds
// outbound/client tls.Config — see LoadClientConfig; there is no
// LoadServerConfig with ClientCAs/ClientAuth) plus a matching client-cert
// change on every caller (e.g. alt-backend's Connect-RPC client to this
// service), so it is tracked as separate follow-up work rather than folded
// into this fix. In the meantime, augur/handler.go's extractUserID /
// extractTenantID header trust is a known, deferred gap: see their doc
// comments and .claude/rules/security-boundaries.md.
package middleware

import (
	"log/slog"
	"net/http"
	"strings"
)

const PeerIdentityHeader = "X-Alt-Peer-Identity"

type PeerIdentityMiddleware struct {
	allowedCallers map[string]struct{}
	logger         *slog.Logger
}

func NewPeerIdentityMiddleware(allowed []string, logger *slog.Logger) *PeerIdentityMiddleware {
	if logger == nil {
		logger = slog.Default()
	}
	m := &PeerIdentityMiddleware{
		allowedCallers: make(map[string]struct{}, len(allowed)),
		logger:         logger,
	}
	for _, cn := range allowed {
		cn = strings.TrimSpace(cn)
		if cn == "" {
			continue
		}
		m.allowedCallers[cn] = struct{}{}
	}
	return m
}

func (m *PeerIdentityMiddleware) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			m.logger.LogAttrs(r.Context(), slog.LevelWarn,
				"peer_identity: missing mTLS client cert",
				slog.String("path", r.URL.Path),
			)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		cn := r.TLS.PeerCertificates[0].Subject.CommonName
		if _, ok := m.allowedCallers[cn]; !ok {
			m.logger.LogAttrs(r.Context(), slog.LevelWarn,
				"peer_identity: caller not in allowlist",
				slog.String("peer", cn),
				slog.String("path", r.URL.Path),
			)
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		r.Header.Set(PeerIdentityHeader, cn)
		next.ServeHTTP(w, r)
	})
}

func ParseAllowedPeers(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
