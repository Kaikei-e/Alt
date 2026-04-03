package usecase

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type mockVectorEncoder struct {
	embedding []float32
	err       error
}

func (m *mockVectorEncoder) Encode(ctx context.Context, texts []string) ([][]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = m.embedding
	}
	return result, nil
}

func (m *mockVectorEncoder) Version() string { return "mock" }

type mockChunkSearcher struct {
	chunks []ContextItem
	err    error
}

func (m *mockChunkSearcher) SearchByVector(ctx context.Context, embedding []float32, limit int) ([]ContextItem, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.chunks, nil
}

func TestHyDEGenerator_GeneratesAndRetrieves(t *testing.T) {
	chunkID1 := uuid.New()
	chunkID2 := uuid.New()

	llm := &mockPlannerLLM{response: "New York has been a global art hub since the early 20th century, with institutions like MoMA and the Guggenheim."}
	encoder := &mockVectorEncoder{embedding: []float32{0.1, 0.2, 0.3}}
	searcher := &mockChunkSearcher{chunks: []ContextItem{
		{ChunkID: chunkID1, ChunkText: "MoMA was founded in 1929", Title: "MoMA History", Score: 0.85},
		{ChunkID: chunkID2, ChunkText: "NYC art market worth billions", Title: "Art Market", Score: 0.75},
	}}

	hyde := NewHyDEGenerator(llm, encoder, searcher, nil)

	existing := map[string]bool{}
	results, err := hyde.GenerateAndRetrieve(context.Background(), "ニューヨークと芸術のかかわり", existing, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestHyDEGenerator_ExcludesExisting(t *testing.T) {
	existingID := uuid.New()
	newID := uuid.New()

	llm := &mockPlannerLLM{response: "hypothetical answer"}
	encoder := &mockVectorEncoder{embedding: []float32{0.1}}
	searcher := &mockChunkSearcher{chunks: []ContextItem{
		{ChunkID: existingID, ChunkText: "existing", Score: 0.9},
		{ChunkID: newID, ChunkText: "new result", Score: 0.8},
	}}

	hyde := NewHyDEGenerator(llm, encoder, searcher, nil)

	existing := map[string]bool{existingID.String(): true}
	results, err := hyde.GenerateAndRetrieve(context.Background(), "test", existing, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result after exclusion, got %d", len(results))
	}
	if results[0].ChunkID != newID {
		t.Errorf("expected new chunk, got %s", results[0].ChunkID)
	}
}

func TestHyDEGenerator_LLMError_ReturnsEmpty(t *testing.T) {
	llm := &mockPlannerLLM{err: context.DeadlineExceeded}
	encoder := &mockVectorEncoder{embedding: []float32{0.1}}
	searcher := &mockChunkSearcher{chunks: nil}

	hyde := NewHyDEGenerator(llm, encoder, searcher, nil)

	results, err := hyde.GenerateAndRetrieve(context.Background(), "test", nil, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results on LLM failure, got %d", len(results))
	}
}
