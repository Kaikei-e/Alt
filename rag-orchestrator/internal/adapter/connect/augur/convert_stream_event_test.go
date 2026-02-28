package augur

import (
	"io"
	"log/slog"
	"testing"

	"rag-orchestrator/internal/usecase"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHandler() *Handler {
	return &Handler{
		logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
}

func TestConvertStreamEvent_Progress(t *testing.T) {
	h := newTestHandler()

	tests := []struct {
		name    string
		payload string
	}{
		{"searching", "searching"},
		{"generating", "generating"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := usecase.StreamEvent{
				Kind:    usecase.StreamEventKindProgress,
				Payload: tt.payload,
			}

			protoEvent, shouldContinue := h.convertStreamEvent(event)

			require.NotNil(t, protoEvent, "progress event should produce a proto event")
			assert.True(t, shouldContinue, "progress event should continue the stream")
			assert.Equal(t, "progress", protoEvent.Kind)

			// Progress reuses delta payload as carrier
			delta := protoEvent.GetDelta()
			assert.Equal(t, tt.payload, delta)
		})
	}
}

func TestConvertStreamEvent_Heartbeat(t *testing.T) {
	h := newTestHandler()

	event := usecase.StreamEvent{
		Kind:    usecase.StreamEventKindHeartbeat,
		Payload: "",
	}

	protoEvent, shouldContinue := h.convertStreamEvent(event)

	require.NotNil(t, protoEvent, "heartbeat event should produce a proto event")
	assert.True(t, shouldContinue, "heartbeat event should continue the stream")
	assert.Equal(t, "heartbeat", protoEvent.Kind)

	// Heartbeat reuses delta payload as carrier (empty string)
	delta := protoEvent.GetDelta()
	assert.Equal(t, "", delta)
}

func TestConvertStreamEvent_ProgressInvalidPayload(t *testing.T) {
	h := newTestHandler()

	event := usecase.StreamEvent{
		Kind:    usecase.StreamEventKindProgress,
		Payload: 123, // Not a string
	}

	protoEvent, shouldContinue := h.convertStreamEvent(event)

	assert.Nil(t, protoEvent, "invalid payload should produce nil event")
	assert.True(t, shouldContinue, "should continue the stream")
}
