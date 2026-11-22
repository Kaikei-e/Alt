package httpserver

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestNewCopiesAllFieldsVerbatim(t *testing.T) {
	t.Parallel()

	handler := http.NewServeMux()
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13}
	errLog := log.New(io.Discard, "", 0)
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	baseCtxCalled := false
	baseCtx := func(_ net.Listener) context.Context {
		baseCtxCalled = true
		return context.Background()
	}
	connStateCalled := false
	connState := func(_ net.Conn, _ http.ConnState) {
		connStateCalled = true
	}

	srv := New(Config{
		Addr:                         ":8080",
		Handler:                      handler,
		DisableGeneralOptionsHandler: true,
		TLSConfig:                    tlsCfg,
		ReadTimeout:                  15 * time.Second,
		ReadHeaderTimeout:            5 * time.Second,
		WriteTimeout:                 30 * time.Second,
		IdleTimeout:                  120 * time.Second,
		MaxHeaderBytes:               1 << 20,
		ErrorLog:                     errLog,
		BaseContext:                  baseCtx,
		ConnState:                    connState,
		Protocols:                    protocols,
	})

	if srv.Addr != ":8080" {
		t.Fatalf("Addr = %q, want %q", srv.Addr, ":8080")
	}
	if srv.Handler != handler {
		t.Fatal("Handler mismatch")
	}
	if !srv.DisableGeneralOptionsHandler {
		t.Fatal("DisableGeneralOptionsHandler = false, want true")
	}
	if srv.TLSConfig != tlsCfg {
		t.Fatal("TLSConfig mismatch")
	}
	if srv.ReadTimeout != 15*time.Second {
		t.Fatalf("ReadTimeout = %v, want 15s", srv.ReadTimeout)
	}
	if srv.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v, want 5s", srv.ReadHeaderTimeout)
	}
	if srv.WriteTimeout != 30*time.Second {
		t.Fatalf("WriteTimeout = %v, want 30s", srv.WriteTimeout)
	}
	if srv.IdleTimeout != 120*time.Second {
		t.Fatalf("IdleTimeout = %v, want 120s", srv.IdleTimeout)
	}
	if srv.MaxHeaderBytes != 1<<20 {
		t.Fatalf("MaxHeaderBytes = %d, want %d", srv.MaxHeaderBytes, 1<<20)
	}
	if srv.ErrorLog != errLog {
		t.Fatal("ErrorLog mismatch")
	}
	if srv.Protocols != protocols {
		t.Fatal("Protocols mismatch")
	}
	if srv.BaseContext == nil {
		t.Fatal("BaseContext is nil")
	}
	_ = srv.BaseContext(nil)
	if !baseCtxCalled {
		t.Fatal("BaseContext callback was not copied")
	}
	if srv.ConnState == nil {
		t.Fatal("ConnState is nil")
	}
	srv.ConnState(nil, http.StateNew)
	if !connStateCalled {
		t.Fatal("ConnState callback was not copied")
	}
}

func TestNewDoesNotInjectImplicitTimeouts(t *testing.T) {
	t.Parallel()

	srv := New(Config{
		Addr:              ":8888",
		ReadHeaderTimeout: 5 * time.Second,
	})

	if srv.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v, want 5s", srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout != 0 {
		t.Fatalf("ReadTimeout = %v, want 0", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %v, want 0", srv.WriteTimeout)
	}
	if srv.IdleTimeout != 0 {
		t.Fatalf("IdleTimeout = %v, want 0", srv.IdleTimeout)
	}
}
