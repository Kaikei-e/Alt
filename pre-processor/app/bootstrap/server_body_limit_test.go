package bootstrap

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// MED-3a: bodies above 1 MiB must be rejected at the ingress edge so a
// misbehaving caller cannot force the summarize pipeline to buffer giant
// payloads.
func TestHTTPServer_RejectsOversizedBody(t *testing.T) {
	deps := newTestHTTPServer(t, "unit-test-secret")
	srv := NewHTTPServer(deps, false, "")

	oversized := bytes.Repeat([]byte("a"), 2*1024*1024) // 2 MiB
	req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", bytes.NewReader(oversized))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Service-Token", "unit-test-secret")
	res := httptest.NewRecorder()

	srv.ServeHTTP(res, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, res.Code,
		"oversized body must be rejected with 413")
}

func TestHTTPServer_NormalBodyIsAllowed(t *testing.T) {
	deps := newTestHTTPServer(t, "unit-test-secret")
	srv := NewHTTPServer(deps, false, "")

	// Tiny JSON payload must not trigger BodyLimit.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", strings.NewReader(`{"x":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Token", "unit-test-secret")
	res := httptest.NewRecorder()

	srv.ServeHTTP(res, req)

	require.NotEqual(t, http.StatusRequestEntityTooLarge, res.Code,
		"small body must not be blocked by BodyLimit")
}
