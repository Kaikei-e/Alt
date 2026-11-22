// Package httpserver is the canonical constructor for *http.Server across Go
// modules so timeout and listener fields remain explicit at every call site.
package httpserver

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"time"
)

// Config declares every http.Server option explicitly so callers cannot
// silently introduce unrequested timeouts or listener defaults.
type Config struct {
	Addr                         string
	Handler                      http.Handler
	DisableGeneralOptionsHandler bool
	TLSConfig                    *tls.Config
	ReadTimeout                  time.Duration
	ReadHeaderTimeout            time.Duration
	WriteTimeout                 time.Duration
	IdleTimeout                  time.Duration
	MaxHeaderBytes               int
	ErrorLog                     *log.Logger
	BaseContext                  func(net.Listener) context.Context
	ConnState                    func(net.Conn, http.ConnState)
	Protocols                    *http.Protocols
}

// New constructs an *http.Server copying every configured field verbatim without
// injecting implicit timeout defaults.
func New(cfg Config) *http.Server {
	return &http.Server{
		Addr:                         cfg.Addr,
		Handler:                      cfg.Handler,
		DisableGeneralOptionsHandler: cfg.DisableGeneralOptionsHandler,
		TLSConfig:                    cfg.TLSConfig,
		ReadTimeout:                  cfg.ReadTimeout,
		ReadHeaderTimeout:            cfg.ReadHeaderTimeout,
		WriteTimeout:                 cfg.WriteTimeout,
		IdleTimeout:                  cfg.IdleTimeout,
		MaxHeaderBytes:               cfg.MaxHeaderBytes,
		ErrorLog:                     cfg.ErrorLog,
		BaseContext:                  cfg.BaseContext,
		ConnState:                    cfg.ConnState,
		Protocols:                    cfg.Protocols,
	}
}
