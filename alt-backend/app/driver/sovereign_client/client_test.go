package sovereign_client

import (
	"alt/port/knowledge_sovereign_port"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/gen/proto/services/sovereign/v1/sovereignv1connect"
	"connectrpc.com/connect"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSovereignHandler struct {
	sovereignv1connect.UnimplementedKnowledgeSovereignServiceHandler
	lastMutationType string
	lastEntityID     string
	returnErr        error
}

func (m *mockSovereignHandler) ApplyProjectionMutation(
	_ context.Context,
	req *connect.Request[sovereignv1.ApplyProjectionMutationRequest],
) (*connect.Response[sovereignv1.ApplyProjectionMutationResponse], error) {
	m.lastMutationType = req.Msg.MutationType
	m.lastEntityID = req.Msg.EntityId
	if m.returnErr != nil {
		return nil, connect.NewError(connect.CodeInternal, m.returnErr)
	}
	return connect.NewResponse(&sovereignv1.ApplyProjectionMutationResponse{Success: true}), nil
}

func (m *mockSovereignHandler) ApplyRecallMutation(
	_ context.Context,
	req *connect.Request[sovereignv1.ApplyRecallMutationRequest],
) (*connect.Response[sovereignv1.ApplyRecallMutationResponse], error) {
	m.lastMutationType = req.Msg.MutationType
	m.lastEntityID = req.Msg.EntityId
	if m.returnErr != nil {
		return nil, connect.NewError(connect.CodeInternal, m.returnErr)
	}
	return connect.NewResponse(&sovereignv1.ApplyRecallMutationResponse{Success: true}), nil
}

func (m *mockSovereignHandler) ApplyCurationMutation(
	_ context.Context,
	req *connect.Request[sovereignv1.ApplyCurationMutationRequest],
) (*connect.Response[sovereignv1.ApplyCurationMutationResponse], error) {
	m.lastMutationType = req.Msg.MutationType
	m.lastEntityID = req.Msg.EntityId
	if m.returnErr != nil {
		return nil, connect.NewError(connect.CodeInternal, m.returnErr)
	}
	return connect.NewResponse(&sovereignv1.ApplyCurationMutationResponse{Success: true}), nil
}

func setupMockServer(handler *mockSovereignHandler) (*Client, func()) {
	mux := http.NewServeMux()
	path, h := sovereignv1connect.NewKnowledgeSovereignServiceHandler(handler)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)

	client := NewClient(srv.URL, true)
	return client, srv.Close
}

func TestApplyProjectionMutation_SendsCorrectRequest(t *testing.T) {
	handler := &mockSovereignHandler{}
	client, cleanup := setupMockServer(handler)
	defer cleanup()

	payload, _ := json.Marshal(map[string]string{"test": "data"})
	err := client.ApplyProjectionMutation(context.Background(), knowledge_sovereign_port.ProjectionMutation{
		MutationType:   "upsert_home_item",
		EntityID:       "entity-1",
		Payload:        payload,
		IdempotencyKey: "key-1",
	})

	require.NoError(t, err)
	assert.Equal(t, "upsert_home_item", handler.lastMutationType)
	assert.Equal(t, "entity-1", handler.lastEntityID)
}

func TestApplyRecallMutation_SendsCorrectRequest(t *testing.T) {
	handler := &mockSovereignHandler{}
	client, cleanup := setupMockServer(handler)
	defer cleanup()

	err := client.ApplyRecallMutation(context.Background(), knowledge_sovereign_port.RecallMutation{
		MutationType: "snooze_candidate",
		EntityID:     "recall-1",
		Payload:      []byte(`{}`),
	})

	require.NoError(t, err)
	assert.Equal(t, "snooze_candidate", handler.lastMutationType)
	assert.Equal(t, "recall-1", handler.lastEntityID)
}

func TestApplyCurationMutation_SendsCorrectRequest(t *testing.T) {
	handler := &mockSovereignHandler{}
	client, cleanup := setupMockServer(handler)
	defer cleanup()

	err := client.ApplyCurationMutation(context.Background(), knowledge_sovereign_port.CurationMutation{
		MutationType: "dismiss_curation",
		EntityID:     "curation-1",
		Payload:      []byte(`{}`),
	})

	require.NoError(t, err)
	assert.Equal(t, "dismiss_curation", handler.lastMutationType)
	assert.Equal(t, "curation-1", handler.lastEntityID)
}

func TestClient_DisabledNoOps(t *testing.T) {
	client := NewClient("http://unused", false)

	err := client.ApplyProjectionMutation(context.Background(), knowledge_sovereign_port.ProjectionMutation{
		MutationType: "upsert_home_item",
	})
	assert.NoError(t, err)

	err = client.ApplyRecallMutation(context.Background(), knowledge_sovereign_port.RecallMutation{
		MutationType: "snooze_candidate",
	})
	assert.NoError(t, err)

	err = client.ApplyCurationMutation(context.Background(), knowledge_sovereign_port.CurationMutation{
		MutationType: "dismiss_curation",
	})
	assert.NoError(t, err)
}

func TestNewClient_SkipsHealthProbeWhenDisabled(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, false)
	require.NotNil(t, client)
	assert.False(t, client.Enabled(), "client should be disabled")
	assert.Equal(t, int32(0), hits.Load(), "health probe must not run when enabled=false")
}

func TestNewClient_StaysEnabledWhenUpstreamIsConnectCompatible(t *testing.T) {
	// The mock server speaks Connect-RPC. The probe will hit
	// GetActiveProjectionVersion which is not implemented by the mock and
	// returns CodeUnimplemented — that is still a valid Connect response,
	// so the probe must NOT degrade the client.
	handler := &mockSovereignHandler{}
	client, cleanup := setupMockServer(handler)
	defer cleanup()

	assert.True(t, client.Enabled(), "Connect-compatible upstream must keep client enabled")

	// Confirm subsequent mutations actually reach the server.
	err := client.ApplyProjectionMutation(context.Background(), knowledge_sovereign_port.ProjectionMutation{
		MutationType: "upsert_home_item",
		EntityID:     "entity-x",
	})
	require.NoError(t, err)
	assert.Equal(t, "upsert_home_item", handler.lastMutationType)
}

func TestNewClient_DegradesGracefullyOnContentTypeMismatch(t *testing.T) {
	// Simulate the staging-slice misroute scenario: the upstream returns
	// HTTP 200 with application/json instead of a Connect-encoded proto
	// response. The probe must detect this and disable the client so the
	// runtime does NOT enter the listener-error retry loop.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error": "not connect-compatible"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, true)
	require.NotNil(t, client)
	assert.False(t, client.Enabled(),
		"client must degrade to disabled when probe detects content-type mismatch")

	// Subsequent mutations are no-ops, NOT failed RPCs.
	err := client.ApplyProjectionMutation(context.Background(), knowledge_sovereign_port.ProjectionMutation{
		MutationType: "upsert_home_item",
	})
	assert.NoError(t, err, "disabled client must no-op without erroring")
}
