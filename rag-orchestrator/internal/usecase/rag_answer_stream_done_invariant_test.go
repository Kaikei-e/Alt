package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// countDone returns the number of Done events in the stream and the last Done's payload.
func countDone(events []usecase.StreamEvent) (int, *usecase.AnswerWithRAGOutput) {
	count := 0
	var last *usecase.AnswerWithRAGOutput
	for _, e := range events {
		if e.Kind == usecase.StreamEventKindDone {
			count++
			if out, ok := e.Payload.(*usecase.AnswerWithRAGOutput); ok {
				last = out
			}
		}
	}
	return count, last
}

func newDoneInvariantUsecase(retrieve *mockRetrieveContextUsecase, llm *mockLLMClient) usecase.AnswerWithRAGUsecase {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return usecase.NewAnswerWithRAGUsecase(
		retrieve, usecase.NewXMLPromptBuilder(), llm, usecase.NewOutputValidator(0),
		10, 512, 6000, "alpha-v1", "ja", logger,
		usecase.WithQueryClassifier(usecase.NewQueryClassifier(nil, 0)),
	)
}

// drainStream collects every event from a Stream channel.
func drainStream(t *testing.T, ch <-chan usecase.StreamEvent) []usecase.StreamEvent {
	t.Helper()
	var out []usecase.StreamEvent
	for e := range ch {
		out = append(out, e)
	}
	return out
}

// TestDoneInvariant_EmptyQuery: rejecting an empty query previously emitted only
// an Error event and returned. The handler relies on Done to trigger persistence
// (and to know "stream truly ended"), so Done must always fire — even on hard
// validation failures. Done.Answer should be empty so the persist layer skips.
func TestDoneInvariant_EmptyQuery(t *testing.T) {
	ctx := context.Background()
	uc := newDoneInvariantUsecase(new(mockRetrieveContextUsecase), new(mockLLMClient))

	events := drainStream(t, uc.Stream(ctx, usecase.AnswerWithRAGInput{Query: "   "}))

	count, payload := countDone(events)
	assert.Equal(t, 1, count, "exactly one Done must be emitted on empty-query rejection")
	if assert.NotNil(t, payload, "Done payload must be a non-nil *AnswerWithRAGOutput") {
		assert.Equal(t, "", payload.Answer, "Done.Answer must be empty for hard validation failure")
	}
}

// TestDoneInvariant_BuildPromptFailure: when buildPrompt fails (e.g. retriever
// errors), the strategy must still emit a single Done after the Fallback so the
// handler can finalise. Done.Answer should be empty since no LLM output happened.
func TestDoneInvariant_BuildPromptFailure(t *testing.T) {
	ctx := context.Background()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)

	mockRetrieve.On("Execute", mock.Anything, mock.Anything).
		Return(nil, errors.New("retriever exploded"))

	uc := newDoneInvariantUsecase(mockRetrieve, mockLLM)
	events := drainStream(t, uc.Stream(ctx, usecase.AnswerWithRAGInput{Query: "test", UserID: "11111111-1111-4111-8111-111111111111"}))

	count, payload := countDone(events)
	assert.Equal(t, 1, count, "exactly one Done must be emitted on buildPrompt failure")
	if assert.NotNil(t, payload, "Done payload must be *AnswerWithRAGOutput") {
		assert.Equal(t, "", payload.Answer, "Done.Answer must be empty when no deltas were produced")
		assert.True(t, payload.Fallback, "Done.Fallback must be true when fallback path was taken")
	}

	// Order check: Fallback must precede Done (Done is the absolute final event).
	var sawFallback, sawDoneAfter bool
	for _, e := range events {
		if e.Kind == usecase.StreamEventKindFallback {
			sawFallback = true
		}
		if sawFallback && e.Kind == usecase.StreamEventKindDone {
			sawDoneAfter = true
		}
	}
	assert.True(t, sawFallback, "fallback event must be emitted for FE display")
	assert.True(t, sawDoneAfter, "Done must come after Fallback")
}

func TestStream_FallbackTerminals_SetMatchingFallbackCategory(t *testing.T) {
	const userID = "11111111-1111-4111-8111-111111111111"
	chunkID := uuid.New()
	retrievedSingle := &usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkID:         chunkID,
				ChunkText:       "Context about semiconductor logistics and port congestion",
				URL:             "https://example.com/a",
				Title:           "Logistics",
				PublishedAt:     "2026-04-01T00:00:00Z",
				Score:           0.9,
				DocumentVersion: 1,
			},
		},
	}

	cases := []struct {
		name         string
		query        string
		setup        func(retrieve *mockRetrieveContextUsecase, llm *mockLLMClient)
		expectedCode string
	}{
		{
			name:  "buildPrompt relevance insufficient",
			query: "Example Phone X について教えて",
			setup: func(retrieve *mockRetrieveContextUsecase, _ *mockLLMClient) {
				retrieve.On("Execute", mock.Anything, mock.Anything).
					Return(nil, usecase.ErrRelevanceInsufficient)
			},
			expectedCode: usecase.FallbackCodeRelevanceLow,
		},
		{
			name:  "buildPrompt retrieval empty",
			query: "unknown topic",
			setup: func(retrieve *mockRetrieveContextUsecase, _ *mockLLMClient) {
				retrieve.On("Execute", mock.Anything, mock.Anything).
					Return(&usecase.RetrieveContextOutput{Contexts: nil}, nil)
			},
			expectedCode: usecase.FallbackCodeRetrievalEmpty,
		},
		{
			name:  "buildPrompt retrieval failed",
			query: "test query",
			setup: func(retrieve *mockRetrieveContextUsecase, _ *mockLLMClient) {
				retrieve.On("Execute", mock.Anything, mock.Anything).
					Return(nil, errors.New("search backend down"))
			},
			expectedCode: usecase.FallbackCodeRetrievalFailed,
		},
		{
			name:  "chat stream setup error",
			query: "test query",
			setup: func(retrieve *mockRetrieveContextUsecase, llm *mockLLMClient) {
				retrieve.On("Execute", mock.Anything, mock.Anything).Return(retrievedSingle, nil)
				llm.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
					Return(nil, nil, errors.New("ollama unreachable"))
			},
			expectedCode: usecase.FallbackCodeLLMUnavailable,
		},
		{
			name:  "chat stream mid-stream error",
			query: "test query",
			setup: func(retrieve *mockRetrieveContextUsecase, llm *mockLLMClient) {
				retrieve.On("Execute", mock.Anything, mock.Anything).Return(retrievedSingle, nil)
				chunkCh := make(chan domain.LLMStreamChunk)
				errCh := make(chan error, 1)
				errCh <- errors.New("stream reset")
				close(chunkCh)
				close(errCh)
				llm.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
					Return((<-chan domain.LLMStreamChunk)(chunkCh), (<-chan error)(errCh), nil)
			},
			expectedCode: usecase.FallbackCodeLLMStreamFailed,
		},
		{
			name:  "chat stream produced no data",
			query: "test query",
			setup: func(retrieve *mockRetrieveContextUsecase, llm *mockLLMClient) {
				retrieve.On("Execute", mock.Anything, mock.Anything).Return(retrievedSingle, nil)
				chunkCh, errCh := makeLLMStream([]domain.LLMStreamChunk{{Done: true}})
				llm.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
					Return(chunkCh, errCh, nil)
			},
			expectedCode: usecase.FallbackCodeLLMNoOutput,
		},
		{
			name:  "output validation failed",
			query: "test query",
			setup: func(retrieve *mockRetrieveContextUsecase, llm *mockLLMClient) {
				retrieve.On("Execute", mock.Anything, mock.Anything).Return(retrievedSingle, nil)
				chunkCh, errCh := makeLLMStream([]domain.LLMStreamChunk{
					{Response: "not valid json at all"},
					{Done: true},
				})
				llm.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
					Return(chunkCh, errCh, nil)
			},
			expectedCode: usecase.FallbackCodeValidationFailed,
		},
		{
			name:  "model declined answer with empty answer",
			query: "test query",
			setup: func(retrieve *mockRetrieveContextUsecase, llm *mockLLMClient) {
				retrieve.On("Execute", mock.Anything, mock.Anything).Return(retrievedSingle, nil)
				chunkCh, errCh := makeLLMStream([]domain.LLMStreamChunk{
					{Response: `{"answer":"","citations":[],"fallback":true,"reason":"insufficient context"}`},
					{Done: true},
				})
				llm.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
					Return(chunkCh, errCh, nil)
			},
			expectedCode: usecase.FallbackCodeAnswerDeclined,
		},
		{
			name:  "answer rejected after retry for general query",
			query: "AとBの比較について教えて",
			setup: func(retrieve *mockRetrieveContextUsecase, llm *mockLLMClient) {
				retrieve.On("Execute", mock.Anything, mock.Anything).Return(retrievedSingle, nil)
				disclaimer := `{"answer":"The provided context does not contain enough information.","citations":[],"fallback":false,"reason":""}`
				chunkCh, errCh := makeLLMStream([]domain.LLMStreamChunk{
					{Response: disclaimer},
					{Done: true},
				})
				llm.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
					Return(chunkCh, errCh, nil)
				llm.On("Chat", mock.Anything, mock.Anything, mock.Anything).
					Return(&domain.LLMResponse{Text: disclaimer, Done: true}, nil)
			},
			expectedCode: usecase.FallbackCodeAnswerRejected,
		},
		{
			name:  "causal trail weak after retry for causal query",
			query: "なぜ半導体不足が起きたのか？",
			setup: func(retrieve *mockRetrieveContextUsecase, llm *mockLLMClient) {
				retrieve.On("Execute", mock.Anything, mock.Anything).Return(retrievedSingle, nil)
				disclaimer := `{"answer":"The provided context does not contain enough information.","citations":[],"fallback":false,"reason":""}`
				chunkCh, errCh := makeLLMStream([]domain.LLMStreamChunk{
					{Response: disclaimer},
					{Done: true},
				})
				llm.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).
					Return(chunkCh, errCh, nil)
				llm.On("Chat", mock.Anything, mock.Anything, mock.Anything).
					Return(&domain.LLMResponse{Text: disclaimer, Done: true}, nil)
			},
			expectedCode: usecase.FallbackCodeCausalTrailWeak,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockRetrieve := new(mockRetrieveContextUsecase)
			mockLLM := new(mockLLMClient)
			tc.setup(mockRetrieve, mockLLM)

			uc := newDoneInvariantUsecase(mockRetrieve, mockLLM)
			events := drainStream(t, uc.Stream(context.Background(), usecase.AnswerWithRAGInput{
				Query:  tc.query,
				UserID: userID,
			}))

			fallbackEvt := findEvent(events, usecase.StreamEventKindFallback)
			require.NotNil(t, fallbackEvt, "expected StreamEventKindFallback")
			assert.Equal(t, tc.expectedCode, fallbackEvt.Payload, "fallback event payload must match expected code")

			count, doneOut := countDone(events)
			assert.Equal(t, 1, count, "expected exactly one Done event")
			require.NotNil(t, doneOut, "Done payload must not be nil")
			assert.True(t, doneOut.Fallback, "Done.Fallback must be true")
			assert.Equal(t, usecase.FallbackCategory(tc.expectedCode), doneOut.FallbackCategory,
				"Done.FallbackCategory must match the fallback code sent to the client")
		})
	}
}
