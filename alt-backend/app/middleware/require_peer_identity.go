package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"alt/utils/logger"
)

// RequirePeerIdentity is the in-process, fail-closed peer allowlist for the
// data-hub listener. It is deliberately not the same thing as
// PeerIdentityHTTPMiddleware, which only records the caller's CommonName and
// lets an unverified request through — that middleware also serves plaintext
// listeners, so it cannot reject.
//
// Two failure modes it closes:
//
//   - tls.Config.VerifyConnection runs once per handshake. The moment TLS
//     termination moves in front of this process, the handshake-time allowlist
//     stops being consulted and every certificate the shared CA issued is
//     accepted again. Re-checking per request keeps the decision where the
//     request is served.
//   - A request with no verified peer certificate cannot legitimately reach a
//     listener that has no plaintext surface. Reaching this branch means the
//     listener was wired without mTLS, which CLAUDE.md rule 8 makes a panic
//     rather than a 403: a 403 would read as "caller sent a bad cert" and hide
//     the wiring bug behind a plausible client error.
//
// An empty allowlist panics at construction: accepting every CA-issued
// certificate is service B impersonating service A, and it must not be
// reachable by forgetting an environment variable.
func RequirePeerIdentity(allowedPeers []string, next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedPeers))
	for _, p := range allowedPeers {
		if s := strings.TrimSpace(p); s != "" {
			allowed[s] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		panic("RequirePeerIdentity: peer allowlist is empty — refusing to accept every certificate the shared CA issued")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			logger.Logger.LogAttrs(r.Context(), slog.LevelError, "datahub.peer_unverified",
				slog.String("path", r.URL.Path),
				slog.Bool("tls", r.TLS != nil),
			)
			panic("RequirePeerIdentity: request arrived without a verified client certificate — " +
				"the data-hub listener is wired without mutual TLS")
		}

		leaf := r.TLS.PeerCertificates[0]
		candidates := append([]string{leaf.Subject.CommonName}, leaf.DNSNames...)
		for _, c := range candidates {
			if _, ok := allowed[c]; ok {
				next.ServeHTTP(w, r)
				return
			}
		}

		logger.Logger.LogAttrs(r.Context(), slog.LevelError, "datahub.peer_rejected",
			slog.String("peer", leaf.Subject.CommonName),
			slog.Any("dns_names", leaf.DNSNames),
			slog.String("path", r.URL.Path),
		)
		http.Error(w, "forbidden: peer identity not allowed", http.StatusForbidden)
	})
}
