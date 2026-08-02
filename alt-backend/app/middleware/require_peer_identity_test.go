package middleware

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"net/http"
	"net/http/httptest"
	"testing"
)

func peerRequest(t *testing.T, commonName string, dnsNames ...string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/services.datahub.v1.DataHubService/CreateArticle", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{
			Subject:  pkix.Name{CommonName: commonName},
			DNSNames: dnsNames,
		}},
	}
	return req
}

func okHandler(called *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	})
}

// tls.Config.VerifyConnection only runs at handshake time, so it stops
// enforcing the moment TLS termination moves in front of the process. The
// per-request allowlist is what makes data-hub fail closed on identity, and
// unlike middleware/peer_identity.go it rejects rather than only logging.
func TestRequirePeerIdentity_AllowlistDecidesPerRequest(t *testing.T) {
	tests := []struct {
		name        string
		allowed     []string
		req         *http.Request
		wantCode    int
		wantHandler bool
	}{
		{
			name:        "allowlisted common name passes",
			allowed:     []string{"pre-processor", "rag-orchestrator"},
			req:         peerRequest(t, "pre-processor"),
			wantCode:    http.StatusOK,
			wantHandler: true,
		},
		{
			name:        "allowlisted DNS SAN passes when the CN differs",
			allowed:     []string{"rag-orchestrator"},
			req:         peerRequest(t, "leaf-1234", "rag-orchestrator"),
			wantCode:    http.StatusOK,
			wantHandler: true,
		},
		{
			name:        "valid chain but unlisted identity is rejected",
			allowed:     []string{"pre-processor"},
			req:         peerRequest(t, "alt-frontend"),
			wantCode:    http.StatusForbidden,
			wantHandler: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			h := RequirePeerIdentity(tt.allowed, okHandler(&called))

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, tt.req)

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if called != tt.wantHandler {
				t.Errorf("handler invoked = %v, want %v", called, tt.wantHandler)
			}
		})
	}
}

// data-hub has no plaintext listener, so a request without a verified peer
// certificate cannot be a client problem — it means the listener was wired
// without mTLS. CLAUDE.md rule 8: take the loud branch, never the quiet one.
func TestRequirePeerIdentity_PanicsWhenTheRequestHasNoTLSState(t *testing.T) {
	tests := []struct {
		name string
		req  *http.Request
	}{
		{
			name: "plaintext request",
			req:  httptest.NewRequest(http.MethodPost, "/services.datahub.v1.DataHubService/CreateArticle", nil),
		},
		{
			name: "TLS without a client certificate",
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/health", nil)
				r.TLS = &tls.ConnectionState{}
				return r
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			h := RequirePeerIdentity([]string{"pre-processor"}, okHandler(&called))

			defer func() {
				if recover() == nil {
					t.Fatal("expected a panic: an unverified peer means the listener is misconfigured")
				}
				if called {
					t.Error("handler must not run for an unverified peer")
				}
			}()
			h.ServeHTTP(httptest.NewRecorder(), tt.req)
		})
	}
}

// An empty allowlist would accept any certificate the shared CA issued, i.e.
// any service impersonating any other. Refuse to build the middleware at all.
func TestRequirePeerIdentity_PanicsOnEmptyAllowlist(t *testing.T) {
	tests := []struct {
		name    string
		allowed []string
	}{
		{name: "nil", allowed: nil},
		{name: "empty slice", allowed: []string{}},
		{name: "only blanks", allowed: []string{"", "   "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected a panic for an empty peer allowlist")
				}
			}()
			called := false
			_ = RequirePeerIdentity(tt.allowed, okHandler(&called))
		})
	}
}
