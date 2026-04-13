package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"rag-orchestrator/internal/usecase"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
	events := drainStream(t, uc.Stream(ctx, usecase.AnswerWithRAGInput{Query: "test"}))

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
