package infrastructure

import (
	"context"
	"log"
	"log/slog"
	"regexp"
)

// Known log noise: pki-agent's own Docker healthcheck plain-TCP dials the
// reverse-proxy TLS listener to detect a silently dead listener goroutine
// (see cmd/pki-agent/healthcheck.go probeProxy). That dial closes before
// TLS ClientHello, which makes net/http emit
//
//	http: TLS handshake error from 127.0.0.1:<ephemeral>: EOF
//
// on every healthcheck tick (default 15s). The error is expected and carries
// no operational signal, so we drop it at the slog boundary. Any other TLS
// handshake failure — bad cert, wrong SNI, I/O timeout, remote RST — still
// flows through so genuine peer-verification regressions stay observable.
var probeEOFPattern = regexp.MustCompile(`^http: TLS handshake error from 127\.0\.0\.1:\d+: EOF$`)

func isProbeEOF(msg string) bool {
	return probeEOFPattern.MatchString(msg)
}

// filteringHandler wraps a slog.Handler and drops records whose message is
// a known-harmless healthcheck-induced TLS handshake EOF. Every other
// method delegates to the inner handler.
type filteringHandler struct {
	inner slog.Handler
}

func newFilteringHandler(inner slog.Handler) slog.Handler {
	return filteringHandler{inner: inner}
}

func (h filteringHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h filteringHandler) Handle(ctx context.Context, record slog.Record) error {
	if isProbeEOF(record.Message) {
		return nil
	}
	return h.inner.Handle(ctx, record)
}

func (h filteringHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return filteringHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h filteringHandler) WithGroup(name string) slog.Handler {
	return filteringHandler{inner: h.inner.WithGroup(name)}
}

// newProxyErrorLog returns a *log.Logger suitable for http.Server.ErrorLog on
// the mTLS reverse proxy. Output flows through the default slog handler at
// WARN level with the probe-EOF filter applied, so legitimate TLS handshake
// failures surface as structured WARN records and healthcheck noise is muted.
func newProxyErrorLog() *log.Logger {
	return slog.NewLogLogger(newFilteringHandler(slog.Default().Handler()), slog.LevelWarn)
}
