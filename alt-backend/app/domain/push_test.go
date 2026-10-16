package domain

import (
	"strings"
	"testing"
)

func TestValidatePushEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{
			name:     "valid FCM endpoint",
			endpoint: "https://fcm.googleapis.com/fcm/send/dGhpcy1pcy1hLXRlc3Q",
			wantErr:  false,
		},
		{
			name:     "valid FCM subdomain endpoint with explicit 443",
			endpoint: "https://jmt17.fcm.googleapis.com:443/fcm/send/abc",
			wantErr:  false,
		},
		{
			name:     "valid Mozilla autopush endpoint",
			endpoint: "https://updates.push.services.mozilla.com/wpush/v2/gAAAAAB",
			wantErr:  false,
		},
		{
			name:     "valid Apple Web Push endpoint",
			endpoint: "https://web.push.apple.com/QAtR7DUxQ",
			wantErr:  false,
		},
		{
			name:     "valid Windows WNS endpoint",
			endpoint: "https://wns2-par02p.notify.windows.com/w/?token=abc",
			wantErr:  false,
		},
		{
			name:     "empty endpoint rejected",
			endpoint: "",
			wantErr:  true,
		},
		{
			name:     "http scheme rejected",
			endpoint: "http://fcm.googleapis.com/fcm/send/abc",
			wantErr:  true,
		},
		{
			name:     "non-443 port rejected",
			endpoint: "https://fcm.googleapis.com:8443/fcm/send/abc",
			wantErr:  true,
		},
		{
			name:     "userinfo rejected",
			endpoint: "https://user:pass@fcm.googleapis.com/fcm/send/abc",
			wantErr:  true,
		},
		{
			name:     "missing host rejected",
			endpoint: "https:///fcm/send/abc",
			wantErr:  true,
		},
		{
			name:     "internal service SSRF rejected",
			endpoint: "http://search-indexer:9300/v1/search",
			wantErr:  true,
		},
		{
			name:     "metadata IP SSRF rejected",
			endpoint: "https://169.254.169.254/latest/meta-data/",
			wantErr:  true,
		},
		{
			name:     "untrusted external host rejected",
			endpoint: "https://evil.example.com/push/abc",
			wantErr:  true,
		},
		{
			name:     "bare suffix without subdomain rejected",
			endpoint: "https://push.apple.com/abc",
			wantErr:  true,
		},
		{
			name:     "leading dot before suffix rejected",
			endpoint: "https://.push.apple.com/abc",
			wantErr:  true,
		},
		{
			name:     "lookalike suffix rejected",
			endpoint: "https://evil-fcm.googleapis.com/abc",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePushEndpoint(tt.endpoint)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ValidatePushEndpoint(%q) = nil, want error", tt.endpoint)
				}
				if tt.endpoint != "" && strings.Contains(err.Error(), tt.endpoint) {
					t.Errorf("error %q must not echo the capability URL", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidatePushEndpoint(%q) unexpected error: %v", tt.endpoint, err)
			}
		})
	}
}
