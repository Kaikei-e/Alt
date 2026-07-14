package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

// H-001b: The /v1/sse/feeds/stats route must no longer exist on alt-backend.
// Clients have been migrated to the Connect-RPC StreamFeedStats RPC on
// port 9101, which is gated by AuthInterceptor. A residual EventSource
// endpoint without authentication is the exact failure mode H-001 closes.
func TestSSEFeedStatsRouteIsRemoved(t *testing.T) {
	e := echo.New()
	// We do not call any registerSSERoutes function — it has been removed.
	// Just a stub /v1 group so the router has something to fall through.
	e.Group("/v1")

	req := httptest.NewRequest(http.MethodGet, "/v1/sse/feeds/stats", nil)
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)

	require.Equal(t, http.StatusNotFound, res.Code,
		"/v1/sse/feeds/stats must return 404 after the SSE handler is removed")
}
