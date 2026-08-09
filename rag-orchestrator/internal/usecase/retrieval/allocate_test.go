package retrieval_test

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase/retrieval"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestAllocate_DynamicMode_SortsByScore(t *testing.T) {
	sc := &retrieval.StageContext{
		RetrievalID:   "test-1",
		QuotaOriginal: 3,
		QuotaExpanded: 3,
		HitsOriginal: []domain.SearchResult{
			makeSearchResult("EN Article", 0.90),
			makeSearchResult("日本語記事", 0.85),
		},
		HitsExpanded: []retrieval.ContextItem{
			makeContextItem("トップ記事", 0.95),
			makeContextItem("Low Article", 0.40),
		},
	}

	contexts := retrieval.Allocate(sc, retrieval.AllocateConfig{
		DynamicLanguageAllocationEnabled: true,
	}, discardLogger())

	assert.Len(t, contexts, 4)
	assert.Equal(t, "トップ記事", contexts[0].Title)
	assert.Equal(t, "EN Article", contexts[1].Title)
	assert.Equal(t, "日本語記事", contexts[2].Title)
	assert.Equal(t, "Low Article", contexts[3].Title)
}

func TestAllocate_DynamicMode_RespectsQuota(t *testing.T) {
	sc := &retrieval.StageContext{
		RetrievalID:   "test-2",
		QuotaOriginal: 1,
		QuotaExpanded: 1,
		HitsOriginal: []domain.SearchResult{
			makeSearchResult("Article 1", 0.99),
			makeSearchResult("Article 2", 0.80),
		},
		HitsExpanded: []retrieval.ContextItem{
			makeContextItem("Expanded 1", 0.90),
			makeContextItem("Expanded 2", 0.70),
		},
	}

	contexts := retrieval.Allocate(sc, retrieval.AllocateConfig{
		DynamicLanguageAllocationEnabled: true,
	}, discardLogger())

	// totalQuota = 1 + 1 = 2
	assert.Len(t, contexts, 2)
	assert.Equal(t, "Article 1", contexts[0].Title)
	assert.Equal(t, "Expanded 1", contexts[1].Title)
}

func TestAllocate_DynamicMode_DeduplicatesByChunkID(t *testing.T) {
	sharedID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	sc := &retrieval.StageContext{
		RetrievalID:   "test-3",
		QuotaOriginal: 5,
		QuotaExpanded: 5,
		HitsOriginal: []domain.SearchResult{
			{
				Chunk: domain.RagChunk{ID: sharedID, Content: "shared content", CreatedAt: time.Now()},
				Score: 0.90,
				Title: "Shared Article",
			},
		},
		HitsExpanded: []retrieval.ContextItem{
			{ChunkID: sharedID, Title: "Shared Article", Score: 0.85},
			{ChunkID: uuid.New(), Title: "Unique Article", Score: 0.80},
		},
	}

	contexts := retrieval.Allocate(sc, retrieval.AllocateConfig{
		DynamicLanguageAllocationEnabled: true,
	}, discardLogger())

	assert.Len(t, contexts, 2, "duplicate chunk should be removed")
}

func TestAllocate_LegacyMode_PrioritizesEnglish(t *testing.T) {
	sc := &retrieval.StageContext{
		RetrievalID:   "test-4",
		QuotaOriginal: 1,
		QuotaExpanded: 2,
		HitsOriginal: []domain.SearchResult{
			makeSearchResult("Original Article", 0.95),
		},
		HitsExpanded: []retrieval.ContextItem{
			makeContextItem("日本語記事", 0.90),
			makeContextItem("English Article", 0.85),
			makeContextItem("もう一つ", 0.80),
		},
	}

	contexts := retrieval.Allocate(sc, retrieval.AllocateConfig{
		DynamicLanguageAllocationEnabled: false,
	}, discardLogger())

	assert.Len(t, contexts, 3)
	assert.Equal(t, "Original Article", contexts[0].Title)
	// Legacy mode prioritizes English in expanded results
	assert.Equal(t, "English Article", contexts[1].Title)
	// Then fills remaining quota
	assert.Equal(t, "日本語記事", contexts[2].Title)
}

func TestAllocate_EmptyInputs(t *testing.T) {
	sc := &retrieval.StageContext{
		RetrievalID:   "test-5",
		QuotaOriginal: 5,
		QuotaExpanded: 5,
		HitsOriginal:  nil,
		HitsExpanded:  nil,
	}

	contexts := retrieval.Allocate(sc, retrieval.AllocateConfig{
		DynamicLanguageAllocationEnabled: true,
	}, discardLogger())

	assert.Empty(t, contexts)
}

func TestIsJapanese(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"English Title", false},
		{"日本語のタイトル", true},
		{"カタカナ", true},
		{"ひらがな", true},
		{"Mixed日本語Title", true},
		{"123 Numbers", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, retrieval.IsJapanese(tt.input))
		})
	}
}

// Helpers

func makeSearchResult(title string, score float32) domain.SearchResult {
	return domain.SearchResult{
		Chunk: domain.RagChunk{
			ID:        uuid.New(),
			Content:   "content for " + title,
			CreatedAt: time.Now(),
		},
		Score: score,
		Title: title,
	}
}

func makeContextItem(title string, score float32) retrieval.ContextItem {
	return retrieval.ContextItem{
		ChunkID:   uuid.New(),
		ChunkText: "content for " + title,
		Title:     title,
		Score:     score,
	}
}

// TestAllocate_DynamicMode_KeepsBM25OnlyHitsDistinct pins the degraded-mode
// contract. When vector search yields nothing, every hit comes from BM25, and
// BM25 hits are article-level with no chunk id at all — search_indexer_client.go
// sets ChunkID: "" because Meilisearch indexes articles, not chunks. Keying
// dedupe on Chunk.ID alone therefore collapses the whole result set onto
// uuid.Nil, and the relevance gate downstream sees a single context, calls the
// retrieval insufficient, and Augur answers nothing at all. fuseHybridResults
// already keys on article id; this stage must not regress to a coarser one.
func TestAllocate_DynamicMode_KeepsBM25OnlyHitsDistinct(t *testing.T) {
	sc := &retrieval.StageContext{
		RetrievalID:   "bm25-degraded",
		QuotaOriginal: 10,
		QuotaExpanded: 10,
		HitsOriginal: []domain.SearchResult{
			{Chunk: domain.RagChunk{Content: "first"}, ArticleID: "art-1", Title: "Article 1", Score: 0.033},
			{Chunk: domain.RagChunk{Content: "second"}, ArticleID: "art-2", Title: "Article 2", Score: 0.022},
			{Chunk: domain.RagChunk{Content: "third"}, ArticleID: "art-3", Title: "Article 3", Score: 0.011},
		},
	}

	contexts := retrieval.Allocate(sc, retrieval.AllocateConfig{
		DynamicLanguageAllocationEnabled: true,
	}, discardLogger())

	assert.Len(t, contexts, 3,
		"BM25-only hits carry no chunk id and must stay distinct by article instead of collapsing onto uuid.Nil")
}

// TestAllocate_LegacyMode_KeepsBM25OnlyHitsDistinct is the same contract for
// the non-dynamic allocator, which shares the Chunk.ID-keyed dedupe.
func TestAllocate_LegacyMode_KeepsBM25OnlyHitsDistinct(t *testing.T) {
	sc := &retrieval.StageContext{
		RetrievalID:   "bm25-degraded-legacy",
		QuotaOriginal: 10,
		QuotaExpanded: 10,
		HitsOriginal: []domain.SearchResult{
			{Chunk: domain.RagChunk{Content: "first"}, ArticleID: "art-1", Title: "Article 1", Score: 0.033},
			{Chunk: domain.RagChunk{Content: "second"}, ArticleID: "art-2", Title: "Article 2", Score: 0.022},
		},
	}

	contexts := retrieval.Allocate(sc, retrieval.AllocateConfig{
		DynamicLanguageAllocationEnabled: false,
	}, discardLogger())

	assert.Len(t, contexts, 2,
		"BM25-only hits carry no chunk id and must stay distinct by article instead of collapsing onto uuid.Nil")
}

// TestAllocate_StillDeduplicatesRepeatedArticleWithoutChunkID guards the other
// direction: falling back to the article id must still collapse two hits that
// really are the same article.
func TestAllocate_StillDeduplicatesRepeatedArticleWithoutChunkID(t *testing.T) {
	sc := &retrieval.StageContext{
		RetrievalID:   "bm25-dupe",
		QuotaOriginal: 10,
		QuotaExpanded: 10,
		HitsOriginal: []domain.SearchResult{
			{Chunk: domain.RagChunk{Content: "first"}, ArticleID: "art-1", Title: "Article 1", Score: 0.033},
			{Chunk: domain.RagChunk{Content: "first again"}, ArticleID: "art-1", Title: "Article 1", Score: 0.022},
		},
	}

	contexts := retrieval.Allocate(sc, retrieval.AllocateConfig{
		DynamicLanguageAllocationEnabled: true,
	}, discardLogger())

	assert.Len(t, contexts, 1, "the same article without a chunk id is still a duplicate")
}
