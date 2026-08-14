package rest

import (
	"alt/config"
	"alt/di"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/stretchr/testify/require"
)

// Finding [022]: the Augur RAG pair was the only user-facing surface on this
// router without RequireAuth() — `g.GET("/rag/context", …)` went onto the bare
// /v1 group and `e.POST("/sse/v1/rag/answer", …)` straight onto the root Echo
// instance. Anything that can reach :9000 (the compose network, or loopback on
// the host) could pull user-derived knowledge context and spend LLM budget with
// no JWT and no tenant scoping.
//
// Recover() is installed so that an unauthenticated request slipping past a
// missing guard hits the handler with a nil usecase and produces a
// distinguishable 500 (panic recovered) instead of crashing the test binary.
func TestRegisterAugurRoutes_RequiresAuth(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{
			name:   "retrieve context",
			method: http.MethodGet,
			target: "/v1/rag/context?q=ai",
		},
		{
			name:   "answer",
			method: http.MethodPost,
			target: "/sse/v1/rag/answer",
			body:   `{"messages":[{"role":"user","content":"hello"}],"stream":false}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			e.Use(echomiddleware.Recover())
			v1 := e.Group("/v1")
			RegisterAugurRoutes(e, v1, &di.ApplicationComponents{}, &config.Config{})

			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			require.Equal(t, http.StatusUnauthorized, rec.Code,
				"unauthenticated %s %s must be rejected with 401, got %d: %s",
				tt.method, tt.target, rec.Code, rec.Body.String())
		})
	}
}
