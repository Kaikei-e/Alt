package bootstrap

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The compose probe runs `["CMD", "/app-entry", "healthcheck"]`, so argv[1] is
// the whole protocol. Anything else must fall through to the server.
func TestIsHealthcheckInvocation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "compose probe", args: []string{"/app-entry", "healthcheck"}, want: true},
		{name: "no arguments starts the server", args: []string{"/app-entry"}, want: false},
		{name: "empty argv", args: nil, want: false},
		{name: "another subcommand", args: []string{"/app-entry", "serve"}, want: false},
		{name: "healthcheck must be first", args: []string{"/app-entry", "serve", "healthcheck"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsHealthcheckInvocation(tt.args); got != tt.want {
				t.Errorf("IsHealthcheckInvocation(%q) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

// A bind address is not a dial address. compose sets OPS_LISTEN=":9110" so the
// listener answers the container network, and the probe runs inside that same
// container — it has to turn the wildcard back into a loopback target rather
// than trying to dial an empty host.
func TestDialAddr(t *testing.T) {
	tests := []struct {
		name    string
		bind    string
		want    string
		wantErr bool
	}{
		{name: "bare port", bind: ":9110", want: "127.0.0.1:9110"},
		{name: "IPv4 wildcard", bind: "0.0.0.0:9110", want: "127.0.0.1:9110"},
		{name: "IPv6 wildcard", bind: "[::]:9110", want: "[::1]:9110"},
		{name: "explicit loopback is unchanged", bind: "127.0.0.1:9110", want: "127.0.0.1:9110"},
		{name: "IPv6 loopback is unchanged", bind: "[::1]:9110", want: "[::1]:9110"},
		{name: "hostname is unchanged", bind: "localhost:9110", want: "localhost:9110"},
		{name: "routable address is unchanged", bind: "10.0.0.4:9443", want: "10.0.0.4:9443"},
		{name: "no port", bind: "127.0.0.1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DialAddr(tt.bind)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("DialAddr(%q) = %q, want error", tt.bind, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("DialAddr(%q) = %q, want %q", tt.bind, got, tt.want)
			}
		})
	}
}

func TestHealthcheck(t *testing.T) {
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	}))
	defer healthy.Close()

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failing.Close()

	// A listener that accepts and a port that nothing holds, for the
	// TCPTargets half — data-hub's probe has to notice a dead mTLS listener
	// goroutine in an otherwise live process (ADR-000784).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	openAddr := ln.Addr().String()

	closedLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	closedAddr := closedLn.Addr().String()
	_ = closedLn.Close()

	tests := []struct {
		name    string
		opts    HealthcheckOptions
		wantErr bool
	}{
		{
			name: "healthy ops listener",
			opts: HealthcheckOptions{OpsAddr: healthy.Listener.Addr().String()},
		},
		{
			name: "healthy ops listener plus a bound TCP target",
			opts: HealthcheckOptions{
				OpsAddr:    healthy.Listener.Addr().String(),
				TCPTargets: []string{openAddr},
			},
		},
		{
			name:    "ops listener answers non-200",
			opts:    HealthcheckOptions{OpsAddr: failing.Listener.Addr().String()},
			wantErr: true,
		},
		{
			name:    "nothing is listening",
			opts:    HealthcheckOptions{OpsAddr: closedAddr},
			wantErr: true,
		},
		{
			name: "ops listener is up but a TCP target is dead",
			opts: HealthcheckOptions{
				OpsAddr:    healthy.Listener.Addr().String(),
				TCPTargets: []string{closedAddr},
			},
			wantErr: true,
		},
		{
			name:    "unparseable bind address",
			opts:    HealthcheckOptions{OpsAddr: "127.0.0.1"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := Healthcheck(ctx, tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Healthcheck() = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Healthcheck() = %v, want nil", err)
			}
		})
	}
}

// The probe must never outlive the healthcheck `timeout:` compose gives it —
// otherwise Docker kills it and reports a failure whose cause is the probe,
// not the process.
func TestHealthcheckDefaultTimeoutIsBounded(t *testing.T) {
	if healthcheckTimeout <= 0 || healthcheckTimeout > 5*time.Second {
		t.Errorf("healthcheckTimeout = %v, want a positive value no larger than the "+
			"tightest compose healthcheck timeout (5s)", healthcheckTimeout)
	}
}
