package usecase_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// collectStreamEvents drains a StreamEvent channel and returns all events.
func collectStreamEvents(ch <-chan usecase.StreamEvent) []usecase.StreamEvent {
	var events []usecase.StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	return events
}

// findEvent returns the first event matching the kind.
func findEvent(events []usecase.StreamEvent, kind usecase.StreamEventKind) *usecase.StreamEvent {
	for _, e := range events {
		if e.Kind == kind {
			return &e
		}
	}
	return nil
}

// findEvents returns all events matching the kind.
func findEvents(events []usecase.StreamEvent, kind usecase.StreamEventKind) []usecase.StreamEvent {
	var matches []usecase.StreamEvent
	for _, e := range events {
		if e.Kind == kind {
			matches = append(matches, e)
		}
	}
	return matches
}

// setupStreamTest creates the standard test fixtures for stream parser tests.
func setupStreamTest(t *testing.T) (*mockRetrieveContextUsecase, *mockLLMClient, usecase.AnswerWithRAGUsecase, uuid.UUID) {
	t.Helper()
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	builder := usecase.NewXMLPromptBuilder()
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewAnswerWithRAGUsecase(mockRetrieve, builder, mockLLM, usecase.NewOutputValidator(), 10, 512, 6000, "alpha-v1", "ja", testLogger)

	chunkID := uuid.New()
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkID:         chunkID,
				ChunkText:       "Test chunk about AI",
				URL:             "http://example.com/article",
				Title:           "AI Research",
				PublishedAt:     "2025-12-25T00:00:00Z",
				Score:           0.9,
				DocumentVersion: 1,
			},
		},
	}, nil)

	return mockRetrieve, mockLLM, uc, chunkID
}

// makeLLMStream creates chunk and error channels populated with the given response chunks.
func makeLLMStream(chunks []domain.LLMStreamChunk) (<-chan domain.LLMStreamChunk, <-chan error) {
	chunkCh := make(chan domain.LLMStreamChunk, len(chunks))
	errCh := make(chan error)
	for _, c := range chunks {
		chunkCh <- c
	}
	close(chunkCh)
	close(errCh)
	return chunkCh, errCh
}

func TestStreamParser_FullJSONInSingleChunk(t *testing.T) {
	_, mockLLM, uc, chunkID := setupStreamTest(t)

	response := `{"answer": "This is the answer about AI.", "citations": [{"chunk_id":"` + chunkID.String() + `","reason":"relevant"}], "fallback": false, "reason": ""}`
	chunkCh, errCh := makeLLMStream([]domain.LLMStreamChunk{
		{Response: response, Done: false},
		{Done: true},
	})
	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).Return(chunkCh, errCh, nil)

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: "what is AI?"}))

	// Should have delta events containing the answer text
	deltas := findEvents(events, usecase.StreamEventKindDelta)
	assert.NotEmpty(t, deltas, "should have delta events")

	// Combine deltas to reconstruct full streamed answer
	var fullAnswer string
	for _, d := range deltas {
		fullAnswer += d.Payload.(string)
	}
	assert.Equal(t, "This is the answer about AI.", fullAnswer)

	// Should have done event
	doneEvt := findEvent(events, usecase.StreamEventKindDone)
	assert.NotNil(t, doneEvt, "should have done event")
}

func TestStreamParser_JSONChunkedAcrossMultipleTokens(t *testing.T) {
	_, mockLLM, uc, chunkID := setupStreamTest(t)

	// Simulate token-by-token streaming of JSON
	chunks := []domain.LLMStreamChunk{
		{Response: `{"answer": "Hel`},
		{Response: `lo wor`},
		{Response: `ld", "citations`},
		{Response: `": [{"chunk_id":"` + chunkID.String() + `"}], "fall`},
		{Response: `back": false, "reason": ""}`},
		{Done: true},
	}
	chunkCh, errCh := makeLLMStream(chunks)
	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).Return(chunkCh, errCh, nil)

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: "hello"}))

	deltas := findEvents(events, usecase.StreamEventKindDelta)
	assert.NotEmpty(t, deltas, "should progressively emit delta events")

	var fullAnswer string
	for _, d := range deltas {
		fullAnswer += d.Payload.(string)
	}
	assert.Equal(t, "Hello world", fullAnswer)
}

func TestStreamParser_EscapedCharactersInAnswer(t *testing.T) {
	_, mockLLM, uc, _ := setupStreamTest(t)

	// JSON with escaped characters: newlines, quotes, backslashes
	response := `{"answer": "Line 1\nLine 2\n\"quoted\" and C:\\path", "citations": [], "fallback": false, "reason": ""}`
	chunkCh, errCh := makeLLMStream([]domain.LLMStreamChunk{
		{Response: response, Done: false},
		{Done: true},
	})
	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).Return(chunkCh, errCh, nil)

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: "test"}))

	deltas := findEvents(events, usecase.StreamEventKindDelta)
	var fullAnswer string
	for _, d := range deltas {
		fullAnswer += d.Payload.(string)
	}

	assert.Contains(t, fullAnswer, "Line 1\nLine 2")
	assert.Contains(t, fullAnswer, "\"quoted\"")
	assert.Contains(t, fullAnswer, "C:\\path")
}

func TestStreamParser_EscapeSplitAcrossChunks(t *testing.T) {
	_, mockLLM, uc, _ := setupStreamTest(t)

	// The escape sequence \n is split across two chunks: "\" in one, "n" in the next.
	// The streaming parser correctly handles this by buffering the backslash.
	chunks := []domain.LLMStreamChunk{
		{Response: `{"answer": "Line 1\`},
		{Response: `nLine 2", "citations": [], "fallback": false, "reason": ""}`},
		{Done: true},
	}
	chunkCh, errCh := makeLLMStream(chunks)
	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).Return(chunkCh, errCh, nil)

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: "test escape split"}))

	deltas := findEvents(events, usecase.StreamEventKindDelta)
	var fullAnswer string
	for _, d := range deltas {
		fullAnswer += d.Payload.(string)
	}
	// The streaming parser unescapes JSON sequences inline, so \n becomes actual newline
	assert.Contains(t, fullAnswer, "Line 1")
	assert.Contains(t, fullAnswer, "Line 2")

	// Verify the done event completed successfully (answer was validated)
	doneEvt := findEvent(events, usecase.StreamEventKindDone)
	assert.NotNil(t, doneEvt, "stream should complete successfully despite split escape")
}

func TestStreamParser_FallbackResponse(t *testing.T) {
	_, mockLLM, uc, _ := setupStreamTest(t)

	response := `{"answer": "", "citations": [], "fallback": true, "reason": "insufficient context"}`
	chunkCh, errCh := makeLLMStream([]domain.LLMStreamChunk{
		{Response: response, Done: false},
		{Done: true},
	})
	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).Return(chunkCh, errCh, nil)

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: "test"}))

	fallbackEvt := findEvent(events, usecase.StreamEventKindFallback)
	assert.NotNil(t, fallbackEvt, "fallback response should emit fallback event")
}

func TestStreamParser_EmptyQuery(t *testing.T) {
	_, _, uc, _ := setupStreamTest(t)

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: ""}))

	errorEvt := findEvent(events, usecase.StreamEventKindError)
	assert.NotNil(t, errorEvt, "empty query should emit error event")
	assert.Equal(t, "query is required", errorEvt.Payload)
}

func TestStreamParser_WhitespaceQuery(t *testing.T) {
	_, _, uc, _ := setupStreamTest(t)

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: "   "}))

	errorEvt := findEvent(events, usecase.StreamEventKindError)
	assert.NotNil(t, errorEvt, "whitespace-only query should emit error event")
}

func TestStreamParser_LLMStreamSetupError(t *testing.T) {
	_, mockLLM, uc, _ := setupStreamTest(t)

	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil, assert.AnError)

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: "test"}))

	fallbackEvt := findEvent(events, usecase.StreamEventKindFallback)
	assert.NotNil(t, fallbackEvt, "LLM stream setup failure should produce fallback")
}

func TestStreamParser_LLMStreamError(t *testing.T) {
	_, mockLLM, uc, _ := setupStreamTest(t)

	chunkCh := make(chan domain.LLMStreamChunk)
	errCh := make(chan error, 1)
	errCh <- assert.AnError
	close(chunkCh)
	close(errCh)

	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).Return((<-chan domain.LLMStreamChunk)(chunkCh), (<-chan error)(errCh), nil)

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: "test"}))

	fallbackEvt := findEvent(events, usecase.StreamEventKindFallback)
	assert.NotNil(t, fallbackEvt, "LLM stream error should produce fallback")
}

func TestStreamParser_ThinkingEventsForwarded(t *testing.T) {
	_, mockLLM, uc, chunkID := setupStreamTest(t)

	response := `{"answer": "Answer text", "citations": [{"chunk_id":"` + chunkID.String() + `"}], "fallback": false, "reason": ""}`
	chunks := []domain.LLMStreamChunk{
		{Thinking: "Let me think about this...", Response: ""},
		{Thinking: "Analyzing the context...", Response: ""},
		{Response: response},
		{Done: true},
	}
	chunkCh, errCh := makeLLMStream(chunks)
	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).Return(chunkCh, errCh, nil)

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: "test thinking"}))

	thinkingEvents := findEvents(events, usecase.StreamEventKindThinking)
	// First thinking event is the pre-retrieval one, then the two LLM thinking events
	assert.GreaterOrEqual(t, len(thinkingEvents), 3, "should forward thinking events from LLM plus initial thinking")
}

func TestStreamParser_EventSequence(t *testing.T) {
	_, mockLLM, uc, chunkID := setupStreamTest(t)

	response := `{"answer": "The answer", "citations": [{"chunk_id":"` + chunkID.String() + `"}], "fallback": false, "reason": ""}`
	chunkCh, errCh := makeLLMStream([]domain.LLMStreamChunk{
		{Response: response},
		{Done: true},
	})
	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).Return(chunkCh, errCh, nil)

	events := collectStreamEvents(uc.Stream(context.Background(), usecase.AnswerWithRAGInput{Query: "test sequence"}))

	// Verify event sequence order
	var kinds []usecase.StreamEventKind
	for _, e := range events {
		kinds = append(kinds, e.Kind)
	}

	// Expected sequence: thinking → progress(searching) → progress(generating) → meta → delta(s) → done
	assert.Equal(t, usecase.StreamEventKindThinking, kinds[0], "first should be thinking")

	// Find positions of key events
	progressIdx := -1
	metaIdx := -1
	deltaIdx := -1
	doneIdx := -1
	for i, k := range kinds {
		switch k {
		case usecase.StreamEventKindProgress:
			if progressIdx == -1 {
				progressIdx = i
			}
		case usecase.StreamEventKindMeta:
			metaIdx = i
		case usecase.StreamEventKindDelta:
			if deltaIdx == -1 {
				deltaIdx = i
			}
		case usecase.StreamEventKindDone:
			doneIdx = i
		}
	}

	assert.True(t, progressIdx > 0, "progress should come after thinking")
	assert.True(t, metaIdx > progressIdx, "meta should come after progress")
	assert.True(t, deltaIdx > metaIdx, "delta should come after meta")
	assert.True(t, doneIdx > deltaIdx, "done should be last")
}

func TestStreamParser_ContextCancellation(t *testing.T) {
	_, mockLLM, uc, _ := setupStreamTest(t)

	// Create a slow stream that blocks
	chunkCh := make(chan domain.LLMStreamChunk) // unbuffered, will block
	errCh := make(chan error)
	mockLLM.On("ChatStream", mock.Anything, mock.Anything, mock.Anything).Return((<-chan domain.LLMStreamChunk)(chunkCh), (<-chan error)(errCh), nil)

	ctx, cancel := context.WithCancel(context.Background())

	eventCh := uc.Stream(ctx, usecase.AnswerWithRAGInput{Query: "test cancel"})

	// Read the initial thinking event
	<-eventCh

	// Cancel context
	cancel()

	// Channel should drain and close
	events := collectStreamEvents(eventCh)
	_ = events // Just verify it doesn't hang
}
