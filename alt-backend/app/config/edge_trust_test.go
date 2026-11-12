package config

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestValidateBackendListeners_TrustedProxyCIDRs(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		setEnv  bool
		wantErr bool
		wantLog string
	}{
		{
			name:    "unset fails startup",
			setEnv:  false,
			wantErr: true,
		},
		{
			name:    "empty string fails startup",
			env:     "",
			setEnv:  true,
			wantErr: true,
		},
		{
			name:    "whitespace commas fail startup",
			env:     " , , ",
			setEnv:  true,
			wantErr: true,
		},
		{
			name:    "bare ip without prefix fails startup",
			env:     "10.0.0.1",
			setEnv:  true,
			wantErr: true,
		},
		{
			name:    "invalid entry in list fails startup",
			env:     "10.0.0.0/8, not-a-cidr",
			setEnv:  true,
			wantErr: true,
		},
		{
			name:    "explicit none disables proxy trust and logs at startup",
			env:     "none",
			setEnv:  true,
			wantErr: false,
			wantLog: "none",
		},
		{
			name:    "valid ipv4 and ipv6 cidrs succeed and log at startup",
			env:     "10.0.0.0/8, 172.16.0.0/12, fd00::/8",
			setEnv:  true,
			wantErr: false,
			wantLog: "10.0.0.0/8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TRUSTED_PROXY_CIDRS", "")
			if tt.setEnv {
				t.Setenv("TRUSTED_PROXY_CIDRS", tt.env)
			} else {
				if err := os.Unsetenv("TRUSTED_PROXY_CIDRS"); err != nil {
					t.Fatalf("Unsetenv failed: %v", err)
				}
				if _, ok := os.LookupEnv("TRUSTED_PROXY_CIDRS"); ok {
					t.Fatal("expected TRUSTED_PROXY_CIDRS to be unset")
				}
			}

			var logBuf bytes.Buffer
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil)))
			defer slog.SetDefault(prev)

			err := ValidateBackendListeners(baseConfig())
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for TRUSTED_PROXY_CIDRS=%q, got nil", tt.env)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantLog != "" && !strings.Contains(logBuf.String(), tt.wantLog) {
				t.Errorf("startup log %q did not mention %q", logBuf.String(), tt.wantLog)
			}
		})
	}
}
