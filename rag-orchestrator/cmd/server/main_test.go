package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

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
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	pinger := &stubHealthPinger{}
	mux := newHealthMux(pinger, log)

	t.Run("/healthz returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
	})

	t.Run("/readyz returns 200 when pinger succeeds", func(t *testing.T) {
		pinger.err = nil
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"status":"ready"}`, rec.Body.String())
	})

	t.Run("/readyz returns 503 when pinger fails", func(t *testing.T) {
		pinger.err = errors.New("db unreachable")
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.JSONEq(t, `{"status":"db down"}`, rec.Body.String())
	})

	t.Run("/metrics returns 200 with Prometheus payload", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "go_goroutines")
	})
}
