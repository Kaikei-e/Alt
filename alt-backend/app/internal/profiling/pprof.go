// Package profiling owns the pprof listener and, with it, the only import of
// net/http/pprof in the module.
//
// That import registers /debug/pprof/* on http.DefaultServeMux as a side
// effect. Keeping it in one package that only cmd/backend imports means
// cmd/harvester and cmd/datahub do not link it at all, so a future health
// endpoint written as http.ListenAndServe(addr, nil) in either of those
// binaries cannot accidentally serve heap and goroutine dumps.
package profiling

import (
	"context"
	"log/slog"
	"net/http"
	_ "net/http/pprof" //#nosec G108 -- registered on our own mux; the listener only starts when PPROF_ENABLED=true
	"os"
	"runtime"
	"time"
)

// Server is a running pprof listener, or nil when profiling is disabled.
type Server struct {
	srv *http.Server
}

// Start starts the pprof listener when PPROF_ENABLED=true, and returns nil
// otherwise. Close is safe to call on the nil result.
func Start(ctx context.Context, log *slog.Logger) *Server {
	if os.Getenv("PPROF_ENABLED") != "true" {
		return nil
	}

	runtime.SetMutexProfileFraction(5)
	runtime.SetBlockProfileRate(1)

	srv := &http.Server{
		Addr:              ":6060",
		Handler:           http.DefaultServeMux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// Profile downloads (a 30s CPU profile in particular) outlive any
		// fixed write deadline.
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}
	go func() { _ = srv.ListenAndServe() }()
	log.InfoContext(ctx, "pprof server started", "port", 6060)

	return &Server{srv: srv}
}

// Close stops the listener. A nil receiver means profiling was never enabled.
func (s *Server) Close() {
	if s == nil || s.srv == nil {
		return
	}
	_ = s.srv.Close()
}
