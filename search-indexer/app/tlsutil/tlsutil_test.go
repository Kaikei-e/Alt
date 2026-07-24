package tlsutil

import (
	"net/http"
	"testing"
)

// TestNewMTLSHTTPServer_SetsSlowlorisTimeouts reproduces the MED finding:
// NewMTLSHTTPServer only set ReadHeaderTimeout and IdleTimeout, leaving
// ReadTimeout, WriteTimeout, and MaxHeaderBytes unset (zero value = no
// bound), unlike the plaintext servers in bootstrap/servers.go which set
// all five. An unbounded ReadTimeout/WriteTimeout on the mTLS listener
// (:9443, REST + Connect-RPC) leaves it open to slow-request /
// slow-response connection exhaustion (CWE-400).
func TestNewMTLSHTTPServer_SetsSlowlorisTimeouts(t *testing.T) {
	srv := NewMTLSHTTPServer(":9443", nil, http.NewServeMux())

	if srv.ReadTimeout <= 0 {
		t.Errorf("ReadTimeout = %v, want > 0", srv.ReadTimeout)
	}
	if srv.WriteTimeout <= 0 {
		t.Errorf("WriteTimeout = %v, want > 0", srv.WriteTimeout)
	}
	if srv.MaxHeaderBytes <= 0 {
		t.Errorf("MaxHeaderBytes = %v, want > 0", srv.MaxHeaderBytes)
	}
	if srv.ReadHeaderTimeout <= 0 {
		t.Errorf("ReadHeaderTimeout = %v, want > 0", srv.ReadHeaderTimeout)
	}
	if srv.IdleTimeout <= 0 {
		t.Errorf("IdleTimeout = %v, want > 0", srv.IdleTimeout)
	}
	if srv.ReadTimeout < srv.ReadHeaderTimeout {
		t.Errorf("ReadTimeout = %v must be >= ReadHeaderTimeout = %v", srv.ReadTimeout, srv.ReadHeaderTimeout)
	}
}
