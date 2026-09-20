package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/config"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
	"knowledge-sovereign/gen/proto/services/sovereign/v1/sovereignv1connect"
	"knowledge-sovereign/handler"
)

type testSovereignHandler struct {
	sovereignv1connect.UnimplementedKnowledgeSovereignServiceHandler
}

func (testSovereignHandler) AppendKnowledgeEvent(
	_ context.Context,
	_ *connect.Request[sovereignv1.AppendKnowledgeEventRequest],
) (*connect.Response[sovereignv1.AppendKnowledgeEventResponse], error) {
	return connect.NewResponse(&sovereignv1.AppendKnowledgeEventResponse{EventSeq: 1}), nil
}

func setupTestServer(token string, enabled bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handler.HealthHandler)
	path, rpcHandler := sovereignv1connect.NewKnowledgeSovereignServiceHandler(
		testSovereignHandler{},
		connect.WithInterceptors(handler.NewEventAuthInterceptor(token, enabled)),
	)
	mux.Handle(path, rpcHandler)
	return mux
}

func TestEventAuth_EnabledGatesRPCPaths(t *testing.T) {
	const validToken = "super-secret-event-token-value"
	h := setupTestServer(validToken, true)

	cases := []struct {
		name     string
		path     string
		auth     string
		wantCode int
	}{
		{
			name:     "RPC without token",
			path:     "/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent",
			auth:     "",
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "RPC with wrong token",
			path:     "/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent",
			auth:     "Bearer wrong-token",
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "RPC with right token",
			path:     "/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent",
			auth:     "Bearer " + validToken,
			wantCode: http.StatusOK,
		},
		{
			name:     "non-RPC path stays unauthenticated",
			path:     "/health",
			auth:     "",
			wantCode: http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}

// An empty token with the gate enabled must deny, never fall open.
func TestEventAuth_EmptyTokenWhileEnabledDenies(t *testing.T) {
	h := setupTestServer("", true)

	req := httptest.NewRequest(
		http.MethodPost,
		"/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent",
		strings.NewReader(`{}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestEventAuth_ExplicitlyDisabledPassesThrough(t *testing.T) {
	h := setupTestServer("", false)

	req := httptest.NewRequest(
		http.MethodPost,
		"/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent",
		strings.NewReader(`{}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestEventAuth_StartupFailureWhenNeitherSet(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ADMIN_AUTH", "disabled")
	t.Setenv("EVENT_TOKEN", "")
	t.Setenv("EVENT_TOKEN_FILE", "")
	t.Setenv("EVENT_AUTH", "")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EVENT_TOKEN")
}
