package middleware

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"net/http"
	"net/http/httptest"
	"testing"
)

func capturePeerHeader(got *string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = r.Header.Get(PeerIdentityHeader)
		w.WriteHeader(http.StatusOK)
	})
}

func verifiedTLSState(cn string) *tls.ConnectionState {
	return &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{Subject: pkix.Name{CommonName: cn}}},
	}
}

// The plaintext listeners take this handler too, and nothing there verifies a
// certificate. A caller-supplied header must not survive into the request the
// handlers see, or the audit trail records whoever the caller claimed to be.
func TestPeerIdentityHTTPMiddleware_StripsCallerSuppliedHeader(t *testing.T) {
	res := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/internal/system-user", nil)
	req.Header.Set(PeerIdentityHeader, "alt-backend")

	var got string
	PeerIdentityHTTPMiddleware(capturePeerHeader(&got)).ServeHTTP(res, req)

	if got != "" {
		t.Errorf("peer identity header on a plaintext request = %q, want it stripped", got)
	}
}

func TestPeerIdentityHTTPMiddleware_OverwritesForgedHeaderWithCertCN(t *testing.T) {
	res := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/internal/system-user", nil)
	req.Header.Set(PeerIdentityHeader, "auth-hub")
	req.TLS = verifiedTLSState("pre-processor")

	var got string
	PeerIdentityHTTPMiddleware(capturePeerHeader(&got)).ServeHTTP(res, req)

	if got != "pre-processor" {
		t.Errorf("peer identity header = %q, want the client certificate CN", got)
	}
}
