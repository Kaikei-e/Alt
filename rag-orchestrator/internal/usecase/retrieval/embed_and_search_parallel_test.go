package retrieval_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase/retrieval"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// instrumentedBM25Searcher records how many SearchBM25 calls are in flight at
// once. With barrierAt > 0 it blocks every call until that many are running
// concurrently, so a sequential caller can only reach the guard timeout.
type instrumentedBM25Searcher struct {
	mu          sync.Mutex
	inFlight    int
	maxInFlight int
	queries     []string

	barrierAt int
	barrier   chan struct{}
	once      sync.Once

	hold     time.Duration
	perQuery map[string][]domain.BM25SearchResult
}

func newInstrumentedBM25Searcher() *instrumentedBM25Searcher {
	return &instrumentedBM25Searcher{
		barrier:  make(chan struct{}),
		perQuery: map[string][]domain.BM25SearchResult{},
	}
}

func (s *instrumentedBM25Searcher) SearchBM25(ctx context.Context, query string, _ int) ([]domain.BM25SearchResult, error) {
	s.mu.Lock()
	s.queries = append(s.queries, query)
	s.inFlight++
	if s.inFlight > s.maxInFlight {
		s.maxInFlight = s.inFlight
	}
	reached := s.barrierAt > 0 && s.inFlight >= s.barrierAt
	s.mu.Unlock()

	if reached {
		s.once.Do(func() { close(s.barrier) })
	}
	if s.barrierAt > 0 {
		select {
		case <-s.barrier:
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}
	if s.hold > 0 {
		timer := time.NewTimer(s.hold)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
		}
	}

	s.mu.Lock()
	s.inFlight--
	s.mu.Unlock()

	return s.perQuery[query], nil
}

func (s *instrumentedBM25Searcher) peakConcurrency() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxInFlight
}

// TestEmbedAndSearch_BM25QueriesRunConcurrently: query expansion produces up to
// eight extra queries, and the lexical arm used to issue all nine as sequential
// HTTP round-trips to search-indexer, serialising the slowest stage of the
// pipeline behind itself.
func TestEmbedAndSearch_BM25QueriesRunConcurrently(t *testing.T) {
	searcher := newInstrumentedBM25Searcher()
	searcher.barrierAt = 3

	sc := &retrieval.StageContext{
		RetrievalID:     "bm25-parallel",
		Query:           "original",
		ExpandedQueries: []string{"expanded one", "expanded two"},
		SearchLimit:     50,
	}

	encoder := new(MockVectorEncoder)
	encoder.On("Encode", mock.Anything, mock.Anything).Return([][]float32{{0.1}, {0.2}}, nil)

	err := retrieval.EmbedAndSearch(context.Background(), sc, encoder, searcher, nil,
		new(MockRagChunkRepository), true, 50, discardLogger())
	require.NoError(t, err)

	assert.Equal(t, 3, searcher.peakConcurrency(),
		"all BM25 queries must be in flight together; a sequential arm never gets past one")
}

// TestEmbedAndSearch_BM25ConcurrencyIsBounded keeps the fan-out from turning a
// nine-query expansion into nine simultaneous connections to search-indexer.
func TestEmbedAndSearch_BM25ConcurrencyIsBounded(t *testing.T) {
	searcher := newInstrumentedBM25Searcher()
	searcher.hold = 30 * time.Millisecond

	expanded := []string{"q1", "q2", "q3", "q4", "q5", "q6", "q7", "q8"}
	sc := &retrieval.StageContext{
		RetrievalID:     "bm25-bounded",
		Query:           "original",
		ExpandedQueries: expanded,
		SearchLimit:     50,
	}

	encoder := new(MockVectorEncoder)
	encoder.On("Encode", mock.Anything, mock.Anything).Return(make([][]float32, len(expanded)), nil)

	err := retrieval.EmbedAndSearch(context.Background(), sc, encoder, searcher, nil,
		new(MockRagChunkRepository), true, 50, discardLogger())
	require.NoError(t, err)

	peak := searcher.peakConcurrency()
	assert.Greater(t, peak, 1, "the arm must actually parallelise")
	assert.LessOrEqual(t, peak, retrieval.BM25MaxConcurrency,
		"unbounded fan-out to search-indexer is not parallelism, it is a thundering herd")
}

// TestEmbedAndSearch_BM25ParallelPreservesQueryAttribution: parallelising the
// arm must not shuffle which query produced which hit. The merged list stays in
// query order, and first-seen wins on duplicates, exactly as the sequential
// loop did.
func TestEmbedAndSearch_BM25ParallelPreservesQueryAttribution(t *testing.T) {
	searcher := newInstrumentedBM25Searcher()
	searcher.hold = 10 * time.Millisecond
	searcher.perQuery = map[string][]domain.BM25SearchResult{
		"original": {
			{ArticleID: "art-a", Title: "A", Rank: 1},
			{ArticleID: "art-b", Title: "B", Rank: 2},
		},
		"second": {
			{ArticleID: "art-b", Title: "B", Rank: 1},
			{ArticleID: "art-c", Title: "C", Rank: 2},
		},
		"third": {
			{ArticleID: "art-d", Title: "D", Rank: 1},
		},
	}

	sc := &retrieval.StageContext{
		RetrievalID:     "bm25-attribution",
		Query:           "original",
		ExpandedQueries: []string{"second", "third"},
		SearchLimit:     50,
	}

	encoder := new(MockVectorEncoder)
	encoder.On("Encode", mock.Anything, mock.Anything).Return([][]float32{{0.1}, {0.2}}, nil)

	err := retrieval.EmbedAndSearch(context.Background(), sc, encoder, searcher, nil,
		new(MockRagChunkRepository), true, 50, discardLogger())
	require.NoError(t, err)

	got := make([]string, len(sc.BM25Results))
	for i, r := range sc.BM25Results {
		got[i] = r.ArticleID
	}
	assert.Equal(t, []string{"art-a", "art-b", "art-c", "art-d"}, got)
}

// TestEmbedAndSearch_BM25ParallelSkippedUnderCandidateScope re-pins the scope
// rule the parallel rewrite must not lose: SearchBM25 takes no article filter.
func TestEmbedAndSearch_BM25ParallelSkippedUnderCandidateScope(t *testing.T) {
	searcher := newInstrumentedBM25Searcher()

	queryVec := []float32{0.1, 0.2, 0.3}
	sc := &retrieval.StageContext{
		RetrievalID:         "bm25-scoped",
		Query:               "original",
		OriginalEmbedding:   queryVec,
		CandidateArticleIDs: []string{"art-1"},
		SearchLimit:         50,
	}

	repo := new(MockRagChunkRepository)
	repo.On("SearchWithinArticles", mock.Anything, queryVec, sc.CandidateArticleIDs, 50).
		Return([]domain.SearchResult{{Chunk: domain.RagChunk{Content: "scoped"}, ArticleID: "art-1", Score: 0.7, ScoreKind: domain.ScoreKindVector}}, nil)

	err := retrieval.EmbedAndSearch(context.Background(), sc, new(MockVectorEncoder), searcher, nil,
		repo, true, 50, discardLogger())
	require.NoError(t, err)

	assert.Empty(t, searcher.queries, "a corpus-wide lexical arm cannot honour an article scope")
	assert.Len(t, sc.OriginalResults, 1)
}
