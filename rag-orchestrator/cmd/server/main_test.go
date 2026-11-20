package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubHealthPinger struct {
	err error
}

func (s *stubHealthPinger) Ping(context.Context) error {
	return s.err
}

func TestNewHealthMux_ServesHealthReadyAndMetrics(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	pinger := &stubHealthPinger{}
	mux := newHealthMux(pinger, log)

	t.Run("/healthz returns 200", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
	})

	t.Run("/readyz returns 200 when pinger succeeds", func(t *testing.T) {
		pinger.err = nil
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"status":"ready"}`, rec.Body.String())
	})

	t.Run("/readyz returns 503 when pinger fails", func(t *testing.T) {
		pinger.err = errors.New("db unreachable")
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.JSONEq(t, `{"status":"db down"}`, rec.Body.String())
	})

	t.Run("/metrics returns 200 with Prometheus payload", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "go_goroutines")
	})
}

func TestPlaintextConnectServer_H2CStreamOutlivesReadHeaderTimeout(t *testing.T) {
	const (
		scaledHeaderTimeout = 200 * time.Millisecond
		chunkCount          = 6
		chunkInterval       = 100 * time.Millisecond
		chunk               = "chunk\n"
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		for range chunkCount {
			if _, err := io.WriteString(w, chunk); err != nil {
				return
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			time.Sleep(chunkInterval)
		}
	})

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := newPlaintextConnectServer(ln.Addr().String(), handler)
	srv.ReadHeaderTimeout = scaledHeaderTimeout

	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	p := new(http.Protocols)
	p.SetUnencryptedHTTP2(true)
	tr := &http.Transport{Protocols: p}
	t.Cleanup(tr.CloseIdleConnections)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+ln.Addr().String()+"/stream", nil)
	require.NoError(t, err)

	resp, err := tr.RoundTrip(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "h2c stream died after %d of %d chunks", strings.Count(string(body), chunk), chunkCount)
	assert.Equal(t, chunkCount, strings.Count(string(body), chunk))
}
