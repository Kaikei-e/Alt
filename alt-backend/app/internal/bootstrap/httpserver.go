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
// ReadTimeout and WriteTimeout are deliberately 0 (unbounded): server-streaming
// responses outlive any fixed write deadline, and client-streaming uploads
// outlive any fixed read deadline. ReadHeaderTimeout still bounds the
// slow-header attack, and IdleTimeout still reaps idle keep-alives, so
// "unbounded" applies only to an in-flight stream. Stated explicitly rather
// than left at the zero value so the choice is visible (bp-go rule 15).
func NewConnectServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       0,
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
