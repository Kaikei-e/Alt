// Package middleware contains HTTP middleware for rag-orchestrator.
//
// PeerIdentityMiddleware reads the TLS client-cert CommonName from r.TLS and
// enforces an allowlist. It is wired by cmd/server/main.go when
// PEER_IDENTITY_MODE=mtls: the Connect-RPC listener then terminates TLS with
// RequireAndVerifyClientCert (tlsutil.LoadServerConfig) and Require() gates
// every RPC, so r.TLS.PeerCertificates is a CA-verified peer by the time the
// CN allowlist runs. Require() must only ever be attached to such a TLS
// listener — on a plaintext listener r.TLS is always nil and every request
// would be rejected. With PEER_IDENTITY_MODE=disabled (explicit opt-out) the
// listener stays plaintext h2c, this middleware is not applied, and the
// X-Alt-User-Id trust in augur/handler.go relies on network policy alone
// (see .claude/rules/security-boundaries.md).
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
