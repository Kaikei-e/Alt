package retrieval_test

import (
	"testing"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase/retrieval"

	"github.com/google/uuid"
)

func BenchmarkSelectContextsDynamic_Small(b *testing.B) {
	hitsOriginal, hitsExpanded := generateBenchData(5, 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		retrieval.SelectContextsDynamic(hitsOriginal, hitsExpanded, 7)
	}
}

func BenchmarkSelectContextsDynamic_Large(b *testing.B) {
	hitsOriginal, hitsExpanded := generateBenchData(50, 200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		retrieval.SelectContextsDynamic(hitsOriginal, hitsExpanded, 20)
	}
}

func BenchmarkIsJapanese(b *testing.B) {
	titles := []string{
		"English Only Title",
		"日本語のタイトル",
		"Mixed日本語Title",
		"Another English Title With More Words",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, t := range titles {
			retrieval.IsJapanese(t)
		}
	}
}

func generateBenchData(originalCount, expandedCount int) ([]domain.SearchResult, []retrieval.ContextItem) {
	hitsOriginal := make([]domain.SearchResult, originalCount)
	for i := 0; i < originalCount; i++ {
		hitsOriginal[i] = domain.SearchResult{
			Chunk: domain.RagChunk{
				ID:        uuid.New(),
				Content:   "content from original search result",
				CreatedAt: time.Now(),
			},
			Score: float32(originalCount-i) / float32(originalCount),
			Title: "Article Original",
		}
	}

	hitsExpanded := make([]retrieval.ContextItem, expandedCount)
	for i := 0; i < expandedCount; i++ {
		hitsExpanded[i] = retrieval.ContextItem{
			ChunkID:   uuid.New(),
			ChunkText: "content from expanded search result",
			Title:     "Article Expanded",
			Score:     float32(expandedCount-i) / float32(expandedCount),
		}
	}
	return hitsOriginal, hitsExpanded
}
