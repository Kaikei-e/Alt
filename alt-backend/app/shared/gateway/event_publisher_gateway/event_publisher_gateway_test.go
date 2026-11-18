package event_publisher_gateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
	mqhubv1 "alt/gen/proto/services/mqhub/v1"
	"alt/gen/proto/services/mqhub/v1/mqhubv1connect"
	"alt/shared/driver/mqhub_connect"
	"alt/shared/port/event_publisher_port"
)

type recordingMQHubHandler struct {
	mqhubv1connect.UnimplementedMQHubServiceHandler
	mu       sync.Mutex
	requests []*mqhubv1.PublishRequest
}

func (h *recordingMQHubHandler) Publish(_ context.Context, req *connect.Request[mqhubv1.PublishRequest]) (*connect.Response[mqhubv1.PublishResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requests = append(h.requests, req.Msg)
	return connect.NewResponse(&mqhubv1.PublishResponse{
		MessageId: "msg-001",
		Success:   true,
	}), nil
}

func (h *recordingMQHubHandler) published() []*mqhubv1.PublishRequest {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*mqhubv1.PublishRequest, len(h.requests))
	copy(out, h.requests)
	return out
}

func newTestGateway(t *testing.T, enabled bool) (*EventPublisherGateway, *recordingMQHubHandler) {
	t.Helper()
	handler := &recordingMQHubHandler{}
	mux := http.NewServeMux()
	path, h := mqhubv1connect.NewMQHubServiceHandler(handler)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := mqhub_connect.NewClient(srv.URL, enabled)
	return NewEventPublisherGateway(client, logger), handler
}

func TestPublishSummarizeRequested_ForwardsValidOrigin(t *testing.T) {
	tests := []struct {
		name       string
		origin     domain.SummarizeOrigin
		wantOrigin string
	}{
		{
			name:       "user origin",
			origin:     domain.SummarizeOriginUser,
			wantOrigin: "user",
		},
		{
			name:       "system origin",
			origin:     domain.SummarizeOriginSystem,
			wantOrigin: "system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw, rec := newTestGateway(t, true)

			err := gw.PublishSummarizeRequested(context.Background(), event_publisher_port.SummarizeRequestedEvent{
				ArticleID: "art-002",
				UserID:    "user-001",
				Title:     "Rust Memory Safety",
				Streaming: true,
				Origin:    tt.origin,
			})
			require.NoError(t, err)

			reqs := rec.published()
			require.Len(t, reqs, 1)
			assert.Equal(t, mqhub_connect.StreamKeyArticles, reqs[0].GetStream())
			assert.Equal(t, mqhub_connect.EventTypeSummarizeRequested, reqs[0].GetEvent().GetEventType())

			var payload map[string]any
			require.NoError(t, json.Unmarshal(reqs[0].GetEvent().GetPayload(), &payload))
			assert.Equal(t, "art-002", payload["article_id"])
			assert.Equal(t, "user-001", payload["user_id"])
			assert.Equal(t, "Rust Memory Safety", payload["title"])
			assert.Equal(t, true, payload["streaming"])
			assert.Equal(t, tt.wantOrigin, payload["origin"])
		})
	}
}

func TestPublishSummarizeRequested_RejectsInvalidOrigin(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		origin  domain.SummarizeOrigin
	}{
		{
			name:    "empty origin when enabled",
			enabled: true,
			origin:  "",
		},
		{
			name:    "unknown origin when enabled",
			enabled: true,
			origin:  domain.SummarizeOrigin("unknown"),
		},
		{
			name:    "empty origin when disabled",
			enabled: false,
			origin:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw, rec := newTestGateway(t, tt.enabled)

			err := gw.PublishSummarizeRequested(context.Background(), event_publisher_port.SummarizeRequestedEvent{
				ArticleID: "art-002",
				UserID:    "user-001",
				Title:     "Rust Memory Safety",
				Origin:    tt.origin,
			})
			require.ErrorIs(t, err, domain.ErrInvalidSummarizeOrigin)
			assert.Empty(t, rec.published())
		})
	}
}
