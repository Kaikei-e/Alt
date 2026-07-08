package usecase_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// setupHybridLongFormTest builds the fixtures needed to drive the hybrid
// streaming path (streamHybridLongForm), which Stream() routes to whenever
// deriveAcceptanceProfile resolves strictLongForm=true (detail/synthesis
// intents, or a query containing a "detailed answer" signal phrase).
func setupHybridLongFormTest(t *testing.T) (*mockRetrieveContextUsecase, *mockLLMClient, usecase.AnswerWithRAGUsecase, uuid.UUID, uuid.UUID) {
	t.Helper()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(0),
		10, 512, 6000, "alpha-v1", "ja", testLogger,
		usecase.WithHeartbeatInterval(10*time.Millisecond),
	)

	firstChunkID := uuid.New()
	secondChunkID := uuid.New()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkID: firstChunkID, ChunkText: "Supply chain background", Title: "Supply", Score: 0.9},
			{ChunkID: secondChunkID, ChunkText: "Port congestion detail", Title: "Port", Score: 0.85},
		},
	}, nil)

	return mockRetrieve, mockLLM, uc, firstChunkID, secondChunkID
}

// detailedQuery contains a queryRequestsDetailedAnswer signal phrase so
// deriveAcceptanceProfile resolves strictLongForm=true regardless of intent
// classification, routing Stream() into streamHybridLongForm.
const detailedQuery = "Please explain fully how the supply chain disruption happened"

func TestStreamHybridLongForm_LLMStreamSetupError_EmitsFallback(t *testing.T) {
	_, mockLLM, uc, _, _ := setupHybridLongFormTest(t)

	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, nil, assert.AnError)

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: detailedQuery}))

	fallbackEvt := findEvent(events, usecase.StreamEventKindFallback)
	assert.NotNil(t, fallbackEvt, "ChatStream setup failure should produce a fallback event")

	doneEvt := findEvent(events, usecase.StreamEventKindDone)
	if assert.NotNil(t, doneEvt, "Done must always be emitted") {
		out := doneEvt.Payload.(*usecase.AnswerWithRAGOutput)
		assert.True(t, out.Fallback)
	}
}

func TestStreamHybridLongForm_NoData_EmitsFallback(t *testing.T) {
	_, mockLLM, uc, _, _ := setupHybridLongFormTest(t)

	chunkCh := make(chan domain.LLMStreamChunk, 1)
	errCh := make(chan error)
	chunkCh <- domain.LLMStreamChunk{Done: true}
	close(chunkCh)
	close(errCh)
	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
		Return((<-chan domain.LLMStreamChunk)(chunkCh), (<-chan error)(errCh), nil)

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: detailedQuery}))

	fallbackEvt := findEvent(events, usecase.StreamEventKindFallback)
	if assert.NotNil(t, fallbackEvt, "a stream with no response data should fall back") {
		assert.Equal(t, "llm stream produced no data", fallbackEvt.Payload)
	}

	doneEvt := findEvent(events, usecase.StreamEventKindDone)
	assert.NotNil(t, doneEvt, "Done must always be emitted even on no-data fallback")
}

func TestStreamHybridLongForm_LLMStreamError_EmitsFallback(t *testing.T) {
	_, mockLLM, uc, _, _ := setupHybridLongFormTest(t)

	chunkCh := make(chan domain.LLMStreamChunk)
	errCh := make(chan error, 1)
	errCh <- assert.AnError
	close(chunkCh)
	close(errCh)
	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
		Return((<-chan domain.LLMStreamChunk)(chunkCh), (<-chan error)(errCh), nil)

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: detailedQuery}))

	fallbackEvt := findEvent(events, usecase.StreamEventKindFallback)
	assert.NotNil(t, fallbackEvt, "an errored LLM stream should fall back")
}

func TestStreamHybridLongForm_AcceptedAnswer_EmitsDeltasAndDone(t *testing.T) {
	_, mockLLM, uc, firstChunkID, secondChunkID := setupHybridLongFormTest(t)

	// Long enough (>= 240 runes, the "detail" profile's acceptMinRunes) and
	// backed by 2 citations so it's accepted without triggering a retry.
	longAnswer := strings.Repeat("物流混乱は複数の要因が重なって発生しました。", 12)
	response := `{"answer":"` + longAnswer + `","citations":[{"chunk_id":"` +
		firstChunkID.String() + `","reason":"supply"},{"chunk_id":"` +
		secondChunkID.String() + `","reason":"port"}],"fallback":false,"reason":""}`

	chunkCh := make(chan domain.LLMStreamChunk, 2)
	errCh := make(chan error)
	chunkCh <- domain.LLMStreamChunk{Response: response}
	chunkCh <- domain.LLMStreamChunk{Done: true}
	close(chunkCh)
	close(errCh)
	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
		Return((<-chan domain.LLMStreamChunk)(chunkCh), (<-chan error)(errCh), nil)

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: detailedQuery}))

	deltas := findEvents(events, usecase.StreamEventKindDelta)
	assert.NotEmpty(t, deltas, "hybrid path should emit provisional paragraph-flushed deltas")

	doneEvt := findEvent(events, usecase.StreamEventKindDone)
	if assert.NotNil(t, doneEvt, "Done must always be emitted") {
		out := doneEvt.Payload.(*usecase.AnswerWithRAGOutput)
		assert.False(t, out.Fallback)
		assert.NotEmpty(t, out.Answer)
		assert.Len(t, out.Citations, 2)
	}
}

func TestStreamHybridLongForm_ContextCancellation_DoesNotHang(t *testing.T) {
	_, mockLLM, uc, _, _ := setupHybridLongFormTest(t)

	// Unbuffered channels the mock never writes to again after the first
	// read; ctx cancellation must return promptly instead of blocking on
	// producer goroutines that keep the channels open.
	chunkCh := make(chan domain.LLMStreamChunk)
	errCh := make(chan error)
	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
		Return((<-chan domain.LLMStreamChunk)(chunkCh), (<-chan error)(errCh), nil)

	ctx, cancel := context.WithCancel(context.Background())
	eventCh := uc.Stream(ctx, usecase.AnswerWithRAGInput{Query: detailedQuery})

	// Drain the initial pre-retrieval events before cancelling.
	<-eventCh

	cancel()

	done := make(chan struct{})
	go func() {
		for range eventCh {
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stream did not close its events channel after ctx cancellation")
	}
}
