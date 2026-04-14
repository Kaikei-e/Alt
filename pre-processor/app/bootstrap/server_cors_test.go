package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// MED-2: pre-processor must not echo Access-Control-Allow-Origin for any
// browser origin. CORS is meaningless for a service-to-service internal API
// and a wildcard response would invite cross-origin abuse.
func TestHTTPServer_NoCORSForBrowserOrigins(t *testing.T) {
	deps := newTestHTTPServer(t, "unit-test-secret")
	srv := NewHTTPServer(deps, false, "")

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/summarize", nil)
	req.Header.Set("Origin", "https://evil.test")
	req.Header.Set("Access-Control-Request-Method", "POST")
	res := httptest.NewRecorder()

	srv.ServeHTTP(res, req)

	got := res.Header().Get("Access-Control-Allow-Origin")
	require.Empty(t, got, "pre-processor must not advertise any CORS Allow-Origin; got %q", got)
}

func TestHTTPServer_NoCORSOnSummarizeResponse(t *testing.T) {
	deps := newTestHTTPServer(t, "unit-test-secret")
	srv := NewHTTPServer(deps, false, "")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", nil)
	req.Header.Set("Origin", "https://evil.test")
	res := httptest.NewRecorder()

	srv.ServeHTTP(res, req)

	require.Empty(t, res.Header().Get("Access-Control-Allow-Origin"))
}
