package bootstrap

import (
	"net/http"
	"testing"
	"time"

	"alt/config"
)

func serverTestConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			ReadTimeout:  300 * time.Second,
			WriteTimeout: 300 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
	}
}

// Splitting main.go into three binaries must not quietly re-tune any listener.
// These pin the exact four timeouts each constructor produced in the single
// binary (bp-go rule 9: every http.Server states all four explicitly).
func TestHTTPServerConstructors_PinTimeouts(t *testing.T) {
	cfg := serverTestConfig()
	h := http.NewServeMux()

	tests := []struct {
		name                  string
		srv                   *http.Server
		wantAddr              string
		wantReadHeaderTimeout time.Duration
		wantReadTimeout       time.Duration
		wantWriteTimeout      time.Duration
		wantIdleTimeout       time.Duration
	}{
		{
			name:                  "rest listener",
			srv:                   NewRESTServer(":9000", h, cfg),
			wantAddr:              ":9000",
			wantReadHeaderTimeout: 10 * time.Second,
			wantReadTimeout:       300 * time.Second,
			wantWriteTimeout:      300 * time.Second,
			wantIdleTimeout:       120 * time.Second,
		},
		{
			name:                  "connect listener keeps request reads unbounded for streaming uploads",
			srv:                   NewConnectServer(":9101", h),
			wantAddr:              ":9101",
			wantReadHeaderTimeout: 10 * time.Second,
			wantReadTimeout:       0,
			wantWriteTimeout:      0,
			wantIdleTimeout:       120 * time.Second,
		},
		{
			name:                  "service-to-service listener",
			srv:                   NewServiceServer("127.0.0.1:9102", h, cfg),
			wantAddr:              "127.0.0.1:9102",
			wantReadHeaderTimeout: 10 * time.Second,
			wantReadTimeout:       300 * time.Second,
			wantWriteTimeout:      0,
			wantIdleTimeout:       120 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.srv.Addr != tt.wantAddr {
				t.Errorf("Addr = %q, want %q", tt.srv.Addr, tt.wantAddr)
			}
			if tt.srv.ReadHeaderTimeout != tt.wantReadHeaderTimeout {
				t.Errorf("ReadHeaderTimeout = %v, want %v", tt.srv.ReadHeaderTimeout, tt.wantReadHeaderTimeout)
			}
			if tt.srv.ReadTimeout != tt.wantReadTimeout {
				t.Errorf("ReadTimeout = %v, want %v", tt.srv.ReadTimeout, tt.wantReadTimeout)
			}
			if tt.srv.WriteTimeout != tt.wantWriteTimeout {
				t.Errorf("WriteTimeout = %v, want %v", tt.srv.WriteTimeout, tt.wantWriteTimeout)
			}
			if tt.srv.IdleTimeout != tt.wantIdleTimeout {
				t.Errorf("IdleTimeout = %v, want %v", tt.srv.IdleTimeout, tt.wantIdleTimeout)
			}
		})
	}
}
