package rest

import (
	"alt/config"
	"alt/di"
	"alt/domain"
	"alt/orchestrator/port/rag_integration_port"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
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

type stubRetrieveContextUsecase struct {
	capturedQuery  string
	capturedUserID string
	contexts       []rag_integration_port.RagContext
	err            error
}

func (s *stubRetrieveContextUsecase) Execute(ctx context.Context, query string, userID string) ([]rag_integration_port.RagContext, error) {
	s.capturedQuery = query
	s.capturedUserID = userID
	return s.contexts, s.err
}

func TestAugurHandler_RetrieveContext_Unauthenticated_Returns401(t *testing.T) {
	stubUC := &stubRetrieveContextUsecase{}
	h := NewAugurHandler(stubUC, nil)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/rag/context?q=test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.RetrieveContext(c)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Empty(t, stubUC.capturedUserID, "usecase must not be called when unauthenticated")
}

func TestAugurHandler_RetrieveContext_Authenticated_ForwardsUserID(t *testing.T) {
	userID := uuid.New()
	stubUC := &stubRetrieveContextUsecase{
		contexts: []rag_integration_port.RagContext{
			{Title: "Article 1", Score: 0.9},
		},
	}
	h := NewAugurHandler(stubUC, nil)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/rag/context?q=test", nil)
	userCtx := domain.SetUserContext(req.Context(), &domain.UserContext{
		UserID:    userID,
		TenantID:  uuid.New(),
		Email:     "user@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	req = req.WithContext(userCtx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.RetrieveContext(c)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "test", stubUC.capturedQuery)
	require.Equal(t, userID.String(), stubUC.capturedUserID,
		"authenticated user_id must be forwarded to RetrieveContext usecase")
}
