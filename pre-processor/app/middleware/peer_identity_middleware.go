package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// PeerIdentityHeader is the header downstream handlers use to read the
// authenticated caller's identity (TLS client-cert CommonName).
const PeerIdentityHeader = "X-Alt-Peer-Identity"

// PeerIdentityMiddleware validates that a request arrived over mTLS from a
// caller in the allowlist. The callers are matched by TLS client cert CN.
//
// On mismatch the handler returns 403 Forbidden and the raw CN is logged.
// On match the CN is propagated downstream via X-Alt-Peer-Identity header.
// Requests without verified client certs (r.TLS == nil or empty certs)
// are refused with 401 Unauthorized.
type PeerIdentityMiddleware struct {
	allowedCallers map[string]struct{}
	logger         *slog.Logger
}

// NewPeerIdentityMiddleware returns a middleware that only permits the given
// CNs. If allowed is empty, it fails-closed.
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
		if cn != "" {
			m.allowedCallers[cn] = struct{}{}
		}
	}
	return m
}

// ParseAllowedPeers parses comma-separated allowed peer CNs.
// Defaults to ["alt-backend"] if empty.
func ParseAllowedPeers(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []string{"alt-backend"}
	}
	parts := strings.Split(trimmed, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"alt-backend"}
	}
	return out
}

// Require wraps a standard http.Handler so that only connections with an
// allowed client cert CN reach it.
func (m *PeerIdentityMiddleware) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			m.logger.WarnContext(r.Context(), "peer_identity: missing mTLS client cert", "path", r.URL.Path)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		cn := r.TLS.PeerCertificates[0].Subject.CommonName
		if _, ok := m.allowedCallers[cn]; !ok {
			m.logger.WarnContext(r.Context(), "peer_identity: caller not in allowlist", "peer", cn, "path", r.URL.Path)
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		r.Header.Set(PeerIdentityHeader, cn)
		next.ServeHTTP(w, r)
	})
}

// EchoMiddleware returns an Echo middleware func enforcing peer identity.
func (m *PeerIdentityMiddleware) EchoMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			if req.TLS == nil || len(req.TLS.PeerCertificates) == 0 {
				m.logger.WarnContext(req.Context(), "peer_identity: missing mTLS client cert", "path", req.URL.Path)
				return echo.NewHTTPError(http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			}
			cn := req.TLS.PeerCertificates[0].Subject.CommonName
			if _, ok := m.allowedCallers[cn]; !ok {
				m.logger.WarnContext(req.Context(), "peer_identity: caller not in allowlist", "peer", cn, "path", req.URL.Path)
				return echo.NewHTTPError(http.StatusForbidden, http.StatusText(http.StatusForbidden))
			}
			req.Header.Set(PeerIdentityHeader, cn)
			return next(c)
		}
	}
}
