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
	t.Setenv("EVENT_AUTH_POLICY_FILE", "")
	t.Setenv("SOVEREIGN_AUTH_POLICY_FILE", "")
	t.Setenv("EVENT_AUTH", "")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EVENT_TOKEN")
}

func TestEventAuth_PolicyWiringInServer(t *testing.T) {
	const (
		recapToken   = "recap-token-value-with-length-24-plus"
		legacyToken  = "legacy-shared-token-value-24-plus"
		allowedEvent = "recap.topic_snapshotted.v1"
	)

	cfg := &config.Config{
		EventAuthEnabled: true,
		EventToken:       "", // explicitly cleared when policy is active
		AuthPolicy: &config.AuthPolicy{
			Services: []config.ServicePolicy{
				{
					Name:  "recap-worker",
					Token: recapToken,
					AllowedMethods: map[string]bool{
						"/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent": true,
					},
					AllowedEvents: map[string]bool{
						allowedEvent: true,
					},
				},
			},
		},
	}

	mux := buildRPCMux(testSovereignHandler{}, cfg)

	// Health check stays open
	healthReq := httptest.NewRequest(http.MethodGet, "/health", nil)
	healthRec := httptest.NewRecorder()
	mux.ServeHTTP(healthRec, healthReq)
	assert.Equal(t, http.StatusOK, healthRec.Code)

	// Unauthenticated fails 401
	unauthReq := httptest.NewRequest(http.MethodPost, "/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent", strings.NewReader(`{"event":{"eventType":"`+allowedEvent+`"}}`))
	unauthReq.Header.Set("Content-Type", "application/json")
	unauthRec := httptest.NewRecorder()
	mux.ServeHTTP(unauthRec, unauthReq)
	assert.Equal(t, http.StatusUnauthorized, unauthRec.Code)

	// Legacy shared token fails 401 (no legacy escape when policy enabled)
	legacyReq := httptest.NewRequest(http.MethodPost, "/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent", strings.NewReader(`{"event":{"eventType":"`+allowedEvent+`"}}`))
	legacyReq.Header.Set("Content-Type", "application/json")
	legacyReq.Header.Set("Authorization", "Bearer "+legacyToken)
	legacyRec := httptest.NewRecorder()
	mux.ServeHTTP(legacyRec, legacyReq)
	assert.Equal(t, http.StatusUnauthorized, legacyRec.Code)

	// Valid policy token for allowed event succeeds 200
	validReq := httptest.NewRequest(http.MethodPost, "/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent", strings.NewReader(`{"event":{"eventType":"`+allowedEvent+`"}}`))
	validReq.Header.Set("Content-Type", "application/json")
	validReq.Header.Set("Authorization", "Bearer "+recapToken)
	validRec := httptest.NewRecorder()
	mux.ServeHTTP(validRec, validReq)
	assert.Equal(t, http.StatusOK, validRec.Code)

	// Wrong event type fails 403 PermissionDenied
	wrongEvtReq := httptest.NewRequest(http.MethodPost, "/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent", strings.NewReader(`{"event":{"eventType":"forbidden.event.v1"}}`))
	wrongEvtReq.Header.Set("Content-Type", "application/json")
	wrongEvtReq.Header.Set("Authorization", "Bearer "+recapToken)
	wrongEvtRec := httptest.NewRecorder()
	mux.ServeHTTP(wrongEvtRec, wrongEvtReq)
	assert.Equal(t, http.StatusForbidden, wrongEvtRec.Code)
}
