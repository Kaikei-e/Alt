package config

import (
	"strings"
	"testing"
)

// The backend's :9443 listener carried the user API, the admin API and the
// service-to-service API on one socket, with client-certificate verification
// controlled by an environment variable that defaulted to "off". cmd/datahub
// replaces it with a listener that always verifies. A leftover MTLS_LISTEN=true
// in a compose file would otherwise be read by nobody — the quiet kind of
// failure where the config says one thing and the process does another — so
// the backend refuses to start instead.
func TestRejectBackendMTLSListenerEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{name: "clean environment", env: nil},
		{name: "MTLS_LISTEN left behind", env: map[string]string{"MTLS_LISTEN": "true"}, wantErr: "MTLS_LISTEN"},
		{name: "MTLS_LISTEN=false is still a leftover", env: map[string]string{"MTLS_LISTEN": "false"}, wantErr: "MTLS_LISTEN"},
		{name: "MTLS_PORT left behind", env: map[string]string{"MTLS_PORT": "9443"}, wantErr: "MTLS_PORT"},
		{name: "MTLS_CLIENT_AUTH left behind", env: map[string]string{"MTLS_CLIENT_AUTH": "require_and_verify"}, wantErr: "MTLS_CLIENT_AUTH"},
		{name: "MTLS_ALLOWED_PEERS left behind", env: map[string]string{"MTLS_ALLOWED_PEERS": "pre-processor"}, wantErr: "MTLS_ALLOWED_PEERS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			err := RejectBackendMTLSListenerEnv()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error naming %s", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want it to name %s", err, tt.wantErr)
			}
		})
	}
}

// MTLS_CERT_FILE / MTLS_KEY_FILE / MTLS_CA_FILE stay in use: the backend still
// presents that leaf certificate as a *client* when RAG_ORCHESTRATOR_CONNECT_URL
// is https. Rejecting them would break the RAG hop.
func TestRejectBackendMTLSListenerEnv_KeepsTheClientCertificatePaths(t *testing.T) {
	t.Setenv("MTLS_CERT_FILE", "/etc/alt/tls/tls.crt")
	t.Setenv("MTLS_KEY_FILE", "/etc/alt/tls/tls.key")
	t.Setenv("MTLS_CA_FILE", "/etc/alt/tls/ca.crt")

	if err := RejectBackendMTLSListenerEnv(); err != nil {
		t.Fatalf("client certificate paths must remain valid backend config: %v", err)
	}
}
