package main

import (
	"net/http"
	"testing"
	"time"

	"mq-hub/usecase"
)

func TestNewHTTPServer_WriteTimeoutExceedsMaxTagGenerationWait(t *testing.T) {
	mux := http.NewServeMux()
	srv := newHTTPServer(":9100", mux)

	if srv.Addr != ":9100" {
		t.Errorf("Addr = %q, want %q", srv.Addr, ":9100")
	}
	if srv.Handler != mux {
		t.Error("Handler does not match provided mux")
	}

	maxTagWait := time.Duration(usecase.MaxTagGenerationTimeoutMs) * time.Millisecond
	if srv.WriteTimeout <= maxTagWait {
		t.Fatalf("WriteTimeout = %v, want > %v (MaxTagGenerationTimeoutMs)", srv.WriteTimeout, maxTagWait)
	}

	if srv.ReadHeaderTimeout <= 0 || srv.ReadHeaderTimeout > 10*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want in (0s, 10s]", srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout <= 0 || srv.ReadTimeout > 30*time.Second {
		t.Errorf("ReadTimeout = %v, want in (0s, 30s]", srv.ReadTimeout)
	}
	if srv.IdleTimeout <= 0 {
		t.Errorf("IdleTimeout = %v, want > 0", srv.IdleTimeout)
	}
	if srv.MaxHeaderBytes != 1<<20 {
		t.Errorf("MaxHeaderBytes = %d, want %d", srv.MaxHeaderBytes, 1<<20)
	}
}
