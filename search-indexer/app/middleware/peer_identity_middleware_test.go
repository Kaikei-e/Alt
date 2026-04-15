package middleware

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"net/http"
	"net/http/httptest"
	"testing"
)

func certWithCN(cn string) *x509.Certificate {
	return &x509.Certificate{Subject: pkix.Name{CommonName: cn}}
}

func tlsStateWithCN(cn string) *tls.ConnectionState {
	if cn == "" {
		return nil
	}
	return &tls.ConnectionState{PeerCertificates: []*x509.Certificate{certWithCN(cn)}}
}

func TestPeerIdentity_AllowedCallerPasses(t *testing.T) {
	m := NewPeerIdentityMiddleware([]string{"alt-backend", "rag-orchestrator"})
	h := m.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(PeerIdentityHeader); got != "alt-backend" {
			t.Fatalf("peer header: got %q, want alt-backend", got)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/search", nil)
	req.TLS = tlsStateWithCN("alt-backend")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
}

func TestPeerIdentity_DisallowedCallerRejected(t *testing.T) {
	m := NewPeerIdentityMiddleware([]string{"alt-backend"})
	h := m.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be invoked for disallowed peer")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/search", nil)
	req.TLS = tlsStateWithCN("evil-service")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", rec.Code)
	}
}

func TestPeerIdentity_MissingTLSRejected(t *testing.T) {
	m := NewPeerIdentityMiddleware([]string{"alt-backend"})
	h := m.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be invoked for plaintext request")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/search", nil) // no TLS
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401", rec.Code)
	}
}

func TestPeerIdentity_EmptyAllowlistRejectsAll(t *testing.T) {
	m := NewPeerIdentityMiddleware(nil)
	h := m.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be invoked when allowlist is empty")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/search", nil)
	req.TLS = tlsStateWithCN("alt-backend")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", rec.Code)
	}
}

func TestPeerIdentity_ClientHeaderIsOverwritten(t *testing.T) {
	// A malicious client might try to spoof identity by pre-setting the header.
	// The middleware must overwrite it with the TLS-verified CN.
	m := NewPeerIdentityMiddleware([]string{"alt-backend"})
	var seen string
	h := m.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get(PeerIdentityHeader)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/search", nil)
	req.Header.Set(PeerIdentityHeader, "root") // spoofed
	req.TLS = tlsStateWithCN("alt-backend")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	if seen != "alt-backend" {
		t.Fatalf("peer header: got %q, want alt-backend (must not trust client-supplied value)", seen)
	}
}
