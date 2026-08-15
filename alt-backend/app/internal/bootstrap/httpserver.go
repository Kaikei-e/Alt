package bootstrap

import (
	"errors"
	"net/http"
	"time"

	"alt/config"
)

// readHeaderTimeout is set independently of ReadTimeout on every listener so
// large payload reads stay bounded by ReadTimeout while slow-header
// (Slowloris) attacks are cut off within 10s.
const readHeaderTimeout = 10 * time.Second

// connectIdleTimeout bounds Connect-RPC keep-alive connections.
const connectIdleTimeout = 120 * time.Second

// connectReadTimeout is the request-body read bound of the cleartext HTTP/2
// Connect listener. It MUST stay non-zero; see NewConnectServer. The value
// matches the REST listener's configured default because it bounds the same
// thing there: how long a request body may take to arrive.
const connectReadTimeout = 300 * time.Second

// cleartextHTTP2 is the protocol set every plaintext Connect-RPC listener
// serves: HTTP/1.1 for unary Connect and the browser, plus cleartext HTTP/2
// (h2c) for Connect's gRPC and gRPC-Web protocols, which require it.
//
// This replaces golang.org/x/net/http2/h2c, deprecated in favour of
// http.Server.Protocols (Go 1.24+). The capability is a server field, so it
// cannot be applied by a handler-returning helper — which is why the
// connect/v2 constructors now return the bare mux and the switch is here.
func cleartextHTTP2() *http.Protocols {
	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)
	return p
}

// NewRESTServer builds the browser-facing REST listener. All four timeouts
// come from the server config.
func NewRESTServer(addr string, h http.Handler, cfg *config.Config) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	}
}

// NewConnectServer builds the user-facing Connect-RPC listener.
//
// ReadTimeout MUST be non-zero on a listener that serves cleartext HTTP/2.
// Since Go 1.26.6 net/http arms the ReadHeaderTimeout deadline at the top of
// (*conn).serve, before the h2c preface is checked, so the deadline is already
// on the socket when the connection is handed to the HTTP/2 server — and the
// HTTP/2 server only clears it inside a `ReadTimeout > 0` guard. Leaving
// ReadTimeout at 0 therefore tears every h2c connection down exactly
// ReadHeaderTimeout after it is established, killing all of its multiplexed
// streams mid-flight. With ReadTimeout > 0 the deadline is cleared and
// re-armed per stream against the request body only, so server-streaming
// responses stay unbounded, which is what this listener needs.
//
// WriteTimeout is 0 for that same reason: a server-streaming response outlives
// any fixed write deadline, and unlike ReadTimeout nothing re-scopes it to a
// single stream. IdleTimeout still reaps idle keep-alives.
func NewConnectServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		Protocols:         cleartextHTTP2(),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       connectReadTimeout,
		WriteTimeout:      0,
		IdleTimeout:       connectIdleTimeout,
	}
}

// NewServiceServer builds a service-to-service listener (the backend operator
// surface, the data-hub health surface). Request bodies are bounded by the
// configured ReadTimeout; responses may stream, so WriteTimeout stays 0.
func NewServiceServer(addr string, h http.Handler, cfg *config.Config) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		Protocols:         cleartextHTTP2(),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      0,
		IdleTimeout:       connectIdleTimeout,
	}
}

// isServerClosed reports whether err is the benign "we asked it to stop"
// error every http.Server returns from ListenAndServe after Shutdown.
func isServerClosed(err error) bool {
	return errors.Is(err, http.ErrServerClosed)
}
