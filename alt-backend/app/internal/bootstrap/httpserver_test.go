package bootstrap

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"

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
			// ReadTimeout must stay non-zero here: it is what makes the HTTP/2
			// server clear the leaked ReadHeaderTimeout deadline off an h2c
			// connection and re-scope it to request bodies. A 0 puts the 10s
			// header deadline back on every established stream.
			name:                  "connect listener bounds request bodies so h2c streams survive",
			srv:                   NewConnectServer(":9101", h),
			wantAddr:              ":9101",
			wantReadHeaderTimeout: 10 * time.Second,
			wantReadTimeout:       300 * time.Second,
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

// Since Go 1.26.6 the ReadHeaderTimeout deadline is armed at the top of
// (*http.conn).serve, before the h2c preface check, so it leaks into the
// established HTTP/2 connection; the h2 server only disarms it when
// ReadTimeout > 0. On the Connect listener that tore every h2c connection down
// ReadHeaderTimeout after it was established, killing all multiplexed streams.
//
// This serves the production Connect listener with a time-scaled
// ReadHeaderTimeout and asserts a server-streaming response outlives it.
func TestConnectServer_H2CStreamOutlivesReadHeaderTimeout(t *testing.T) {
	const (
		scaledHeaderTimeout = 500 * time.Millisecond
		chunkCount          = 6
		chunkInterval       = 250 * time.Millisecond
		chunk               = "chunk\n"
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		for range chunkCount {
			if _, err := io.WriteString(w, chunk); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			time.Sleep(chunkInterval)
		}
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := NewConnectServer(ln.Addr().String(), handler)
	srv.ReadHeaderTimeout = scaledHeaderTimeout

	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	tr := &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}
	t.Cleanup(tr.CloseIdleConnections)

	req, err := http.NewRequest(http.MethodGet, "http://"+ln.Addr().String()+"/stream", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("h2c round trip: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("stream died after %d of %d chunks: %v", strings.Count(string(body), chunk), chunkCount, err)
	}
	if got := strings.Count(string(body), chunk); got != chunkCount {
		t.Errorf("received %d chunks, want %d", got, chunkCount)
	}
}
