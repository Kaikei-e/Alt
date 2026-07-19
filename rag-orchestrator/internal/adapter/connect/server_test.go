package connect

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"alt/gen/proto/alt/augur/v2/augurv2connect"

	"rag-orchestrator/internal/middleware"
)

func tlsStateWithCN(cn string) *tls.ConnectionState {
	return &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{Subject: pkix.Name{CommonName: cn}}},
	}
}

func newMTLSHandlerForTest(t *testing.T, allowed []string) http.Handler {
	t.Helper()
	peerMW := middleware.NewPeerIdentityMiddleware(allowed, slog.Default())
	return CreateMTLSConnectServer(peerMW, nil, nil, nil, nil, nil, nil, slog.Default())
}

func TestCreateMTLSConnectServer_RejectsRequestWithoutClientCert(t *testing.T) {
	h := newMTLSHandlerForTest(t, []string{"alt-backend"})

	// The augur procedures read X-Alt-User-Id (extractUserID); a forged
	// header from an unverified peer must be stopped before any handler runs.
	req := httptest.NewRequest(http.MethodPost, augurv2connect.AugurServiceGetConversationProcedure, nil)
	req.Header.Set("X-Alt-User-Id", "b3b8f4a0-0000-4000-8000-000000000000")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401 for plaintext/unverified request", rec.Code)
	}
}

func TestCreateMTLSConnectServer_RejectsDisallowedPeer(t *testing.T) {
	h := newMTLSHandlerForTest(t, []string{"alt-backend"})

	req := httptest.NewRequest(http.MethodPost, augurv2connect.AugurServiceGetConversationProcedure, nil)
	req.TLS = tlsStateWithCN("evil-service")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403 for disallowed peer CN", rec.Code)
	}
}

func TestCreateMTLSConnectServer_AllowedPeerReachesHandlers(t *testing.T) {
	h := newMTLSHandlerForTest(t, []string{"alt-backend"})

	req := httptest.NewRequest(http.MethodGet, "/connect/health", nil)
	req.TLS = tlsStateWithCN("alt-backend")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 for allowed peer", rec.Code)
	}
}

func TestCreateMTLSConnectServer_NilMiddlewarePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("must panic when peer-identity middleware is not wired (no silent fallback)")
		}
	}()
	CreateMTLSConnectServer(nil, nil, nil, nil, nil, nil, nil, slog.Default())
}
