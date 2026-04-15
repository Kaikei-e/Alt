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
	m := NewPeerIdentityMiddleware([]string{"alt-butterfly-facade", "alt-backend"}, nil)
	h := m.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(PeerIdentityHeader); got != "alt-butterfly-facade" {
			t.Fatalf("peer header: got %q, want alt-butterfly-facade", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/answer", nil)
	req.TLS = tlsStateWithCN("alt-butterfly-facade")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
}

func TestPeerIdentity_DisallowedCallerRejected(t *testing.T) {
	m := NewPeerIdentityMiddleware([]string{"alt-butterfly-facade"}, nil)
	h := m.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be invoked for disallowed peer")
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/answer", nil)
	req.TLS = tlsStateWithCN("evil")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", rec.Code)
	}
}

func TestPeerIdentity_MissingTLSRejected(t *testing.T) {
	m := NewPeerIdentityMiddleware([]string{"alt-butterfly-facade"}, nil)
	h := m.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be invoked for plaintext request")
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/answer", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401", rec.Code)
	}
}

func TestPeerIdentity_EmptyAllowlistRejectsAll(t *testing.T) {
	m := NewPeerIdentityMiddleware(nil, nil)
	h := m.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be invoked when allowlist is empty")
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/answer", nil)
	req.TLS = tlsStateWithCN("alt-butterfly-facade")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", rec.Code)
	}
}

func TestPeerIdentity_ClientHeaderIsOverwritten(t *testing.T) {
	m := NewPeerIdentityMiddleware([]string{"alt-butterfly-facade"}, nil)
	var seen string
	h := m.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get(PeerIdentityHeader)
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/answer", nil)
	req.Header.Set(PeerIdentityHeader, "root")
	req.TLS = tlsStateWithCN("alt-butterfly-facade")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if seen != "alt-butterfly-facade" {
		t.Fatalf("peer header: got %q, must not trust spoofed value", seen)
	}
}

func TestParseAllowedPeers(t *testing.T) {
	got := ParseAllowedPeers(" alt-butterfly-facade , alt-backend,  , search-indexer ")
	want := []string{"alt-butterfly-facade", "alt-backend", "search-indexer"}
	if len(got) != len(want) {
		t.Fatalf("len: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] got %q want %q", i, got[i], want[i])
		}
	}
}
