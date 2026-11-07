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
	"github.com/stretchr/testify/require"
)

const testHybridUserID = "11111111-1111-4111-8111-111111111111"

// setupHybridLongFormTest builds the fixtures needed to drive the hybrid
// streaming path (streamHybridLongForm), which Stream() routes to whenever
// deriveAcceptanceProfile resolves strictLongForm=true (detail/synthesis
// intents, or a query containing a "detailed answer" signal phrase).
func setupHybridLongFormTest(t *testing.T) (*mockRetrieveContextUsecase, *mockLLMClient, usecase.AnswerWithRAGUsecase, uuid.UUID, uuid.UUID) {
	t.Helper()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve,
		usecase.NewXMLPromptBuilder(),
		mockLLM,
		usecase.NewOutputValidator(0),
		10, 512, 6000,
		"alpha-v1", "ja",
		testLogger,
		usecase.WithHeartbeatInterval(10*time.Millisecond),
		usecase.WithQueryClassifier(usecase.NewQueryClassifier(nil, 0)),
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

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: detailedQuery, UserID: testHybridUserID}))

	fallbackEvt := findEvent(events, usecase.StreamEventKindFallback)
	if assert.NotNil(t, fallbackEvt, "ChatStream setup failure should produce a fallback event") {
		assert.Equal(t, usecase.FallbackCodeLLMUnavailable, fallbackEvt.Payload)
	}

	doneEvt := findEvent(events, usecase.StreamEventKindDone)
	if assert.NotNil(t, doneEvt, "Done must always be emitted") {
		out := doneEvt.Payload.(*usecase.AnswerWithRAGOutput)
		assert.True(t, out.Fallback)
		assert.Equal(t, usecase.FallbackCategory(usecase.FallbackCodeLLMUnavailable), out.FallbackCategory)
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

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: detailedQuery, UserID: testHybridUserID}))

	fallbackEvt := findEvent(events, usecase.StreamEventKindFallback)
	if assert.NotNil(t, fallbackEvt, "a stream with no response data should fall back") {
		assert.Equal(t, usecase.FallbackCodeLLMNoOutput, fallbackEvt.Payload)
	}

	doneEvt := findEvent(events, usecase.StreamEventKindDone)
	if assert.NotNil(t, doneEvt, "Done must always be emitted even on no-data fallback") {
		out := doneEvt.Payload.(*usecase.AnswerWithRAGOutput)
		assert.True(t, out.Fallback)
		assert.Equal(t, usecase.FallbackCategory(usecase.FallbackCodeLLMNoOutput), out.FallbackCategory)
	}
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

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: detailedQuery, UserID: testHybridUserID}))

	fallbackEvt := findEvent(events, usecase.StreamEventKindFallback)
	if assert.NotNil(t, fallbackEvt, "an errored LLM stream should fall back") {
		assert.Equal(t, usecase.FallbackCodeLLMStreamFailed, fallbackEvt.Payload)
	}

	doneEvt := findEvent(events, usecase.StreamEventKindDone)
	if assert.NotNil(t, doneEvt, "Done must always be emitted") {
		out := doneEvt.Payload.(*usecase.AnswerWithRAGOutput)
		assert.True(t, out.Fallback)
		assert.Equal(t, usecase.FallbackCategory(usecase.FallbackCodeLLMStreamFailed), out.FallbackCategory)
	}
}

func TestStreamHybridLongForm_OtherFallbackTerminals_SetMatchingFallbackCategory(t *testing.T) {
	cases := []struct {
		name         string
		query        string
		rawStream    string
		retryResp    *domain.LLMResponse
		retryErr     error
		expectedCode string
	}{
		{
			name:         "validation failed",
			query:        detailedQuery,
			rawStream:    "not valid json",
			expectedCode: usecase.FallbackCodeValidationFailed,
		},
		{
			name:         "model declined answer with empty answer",
			query:        detailedQuery,
			rawStream:    `{"answer":"","citations":[],"fallback":true,"reason":"insufficient context"}`,
			expectedCode: usecase.FallbackCodeAnswerDeclined,
		},
		{
			name:      "answer rejected after retry",
			query:     "Please explain fully the difference between port A and port B",
			rawStream: `{"answer":"Too short.","citations":[],"fallback":false,"reason":""}`,
			retryResp: &domain.LLMResponse{
				Text: `{"answer":"Still too short.","citations":[],"fallback":false,"reason":""}`,
				Done: true,
			},
			expectedCode: usecase.FallbackCodeAnswerRejected,
		},
		{
			name:      "causal trail weak after retry",
			query:     "なぜ物流混乱が起きたのか詳しく教えてください",
			rawStream: `{"answer":"短すぎる回答。","citations":[],"fallback":false,"reason":""}`,
			retryResp: &domain.LLMResponse{
				Text: `{"answer":"まだ短すぎる回答。","citations":[],"fallback":false,"reason":""}`,
				Done: true,
			},
			expectedCode: usecase.FallbackCodeCausalTrailWeak,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, mockLLM, uc, _, _ := setupHybridLongFormTest(t)

			chunkCh, errCh := makeLLMStream([]domain.LLMStreamChunk{
				{Response: tc.rawStream},
				{Done: true},
			})
			mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
				Return(chunkCh, errCh, nil)
			if tc.retryResp != nil || tc.retryErr != nil {
				mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).
					Return(tc.retryResp, tc.retryErr)
			}

			events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{
				Query:  tc.query,
				UserID: testHybridUserID,
			}))

			fallbackEvt := findEvent(events, usecase.StreamEventKindFallback)
			require.NotNil(t, fallbackEvt, "expected StreamEventKindFallback")
			assert.Equal(t, tc.expectedCode, fallbackEvt.Payload, "fallback event payload must match expected code")

			doneEvt := findEvent(events, usecase.StreamEventKindDone)
			require.NotNil(t, doneEvt, "expected StreamEventKindDone")
			out := doneEvt.Payload.(*usecase.AnswerWithRAGOutput)
			assert.True(t, out.Fallback, "Done.Fallback must be true")
			assert.Equal(t, usecase.FallbackCategory(tc.expectedCode), out.FallbackCategory,
				"Done.FallbackCategory must match the fallback code sent to the client")
		})
	}
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

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: detailedQuery, UserID: testHybridUserID}))

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
	eventCh := uc.Stream(ctx, usecase.AnswerWithRAGInput{Query: detailedQuery, UserID: testHybridUserID})

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
