package usecase

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeNeighborSearcher captures the seed set and query so the tests can
// assert that buildRelatedCitations passes the right arguments through to
// HybridSearcher.SearchNeighbors.
type fakeNeighborSearcher struct {
	lastSeeds  []string
	lastQuery  string
	lastVector []float32
	hits       []domain.SearchResult
	err        error
}

func (f *fakeNeighborSearcher) HybridSearch(_ context.Context, _ []float32, _ string, _ int) ([]domain.SearchResult, error) {
	return nil, nil
}

func (f *fakeNeighborSearcher) SearchNeighbors(_ context.Context, vector []float32, queryText string, seeds []string, _ int) ([]domain.SearchResult, error) {
	f.lastSeeds = append([]string(nil), seeds...)
	f.lastQuery = queryText
	f.lastVector = append([]float32(nil), vector...)
	if f.err != nil {
		return nil, f.err
	}
	return f.hits, nil
}

// fakeNeighborEncoder stands in for the embedder that turns the synthetic
// neighbor query into the vector the pgvector arm needs.
type fakeNeighborEncoder struct {
	vector []float32
	err    error
	calls  int
}

func (f *fakeNeighborEncoder) Encode(_ context.Context, texts []string) ([][]float32, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = f.vector
	}
	return out, nil
}

func (f *fakeNeighborEncoder) Version() string { return "fake/1024" }

func newTestUsecaseWithNeighbor(searcher domain.HybridSearcher) *answerWithRAGUsecase {
	return &answerWithRAGUsecase{
		neighborSearcher: searcher,
		neighborLimit:    3,
		logger:           slog.Default(),
	}
}

// When direct citations carry parseable ArticleIDs, the neighbor seed set
// must be exactly those IDs and the synthetic query must be derived from the
// direct titles — that's the signal that drives a "next-to-read" neighbor.
func TestBuildRelatedCitations_PassesSeedsAndTitleQuery(t *testing.T) {
	a := uuid.New().String()
	b := uuid.New().String()
	fake := &fakeNeighborSearcher{
		hits: []domain.SearchResult{
			{ArticleID: uuid.New().String(), Title: "Neighbor X", URL: "https://x.test"},
			{ArticleID: uuid.New().String(), Title: "Neighbor Y", URL: "https://y.test"},
		},
	}
	u := newTestUsecaseWithNeighbor(fake)

	direct := []Citation{
		{ArticleID: a, Title: "Direct A"},
		{ArticleID: b, Title: "Direct B"},
	}

	related := u.buildRelatedCitations(context.Background(), direct, "user query")
	require.Len(t, related, 2)

	assert.ElementsMatch(t, []string{a, b}, fake.lastSeeds)
	assert.Equal(t, "Direct A Direct B", fake.lastQuery)
	assert.Equal(t, "Neighbor X", related[0].Title)
	assert.Equal(t, "Neighbor Y", related[1].Title)
}

// No direct citations → no neighbor lookup, no related rows. The "if" in the
// user requirement is enforced at this boundary.
func TestBuildRelatedCitations_NoDirectCitations_ReturnsNil(t *testing.T) {
	fake := &fakeNeighborSearcher{}
	u := newTestUsecaseWithNeighbor(fake)

	related := u.buildRelatedCitations(context.Background(), nil, "user query")
	assert.Nil(t, related)
	assert.Nil(t, fake.lastSeeds)
}

// Non-UUID ArticleIDs (and empty ones) are dropped from the seed set; if
// every direct citation lacks a usable ArticleID, neighbor lookup is skipped.
func TestBuildRelatedCitations_NoParseableArticleIDs_ReturnsNil(t *testing.T) {
	fake := &fakeNeighborSearcher{}
	u := newTestUsecaseWithNeighbor(fake)

	direct := []Citation{
		{ArticleID: "", Title: "Web Only"},
		{ArticleID: "not-a-uuid", Title: "Garbage"},
	}

	related := u.buildRelatedCitations(context.Background(), direct, "user query")
	assert.Nil(t, related)
	assert.Nil(t, fake.lastSeeds, "seed list never reaches the searcher when ArticleIDs are unusable")
}

// HybridSearcher errors are absorbed: we log and return an empty result so
// the assistant turn still completes. The "Related" section silently hides;
// the user does not see an error.
func TestBuildRelatedCitations_SearcherError_ReturnsNil(t *testing.T) {
	fake := &fakeNeighborSearcher{err: errors.New("db unavailable")}
	u := newTestUsecaseWithNeighbor(fake)

	direct := []Citation{
		{ArticleID: uuid.New().String(), Title: "Direct"},
	}

	related := u.buildRelatedCitations(context.Background(), direct, "user query")
	assert.Nil(t, related)
}

// The neighbor SQL drops its pgvector arm entirely when the query vector is
// empty, leaving a plainto_tsquery conjunction of every cited title as the only
// arm. On Japanese titles that conjunction matches nothing, which is how
// related_citations stayed empty in every stored assistant turn without a
// single error being logged. Encoding the synthetic query is what makes the
// semantic arm run at all.
func TestBuildRelatedCitations_EncodesQueryVectorForSemanticArm(t *testing.T) {
	fake := &fakeNeighborSearcher{
		hits: []domain.SearchResult{{ArticleID: uuid.New().String(), Title: "Neighbor"}},
	}
	encoder := &fakeNeighborEncoder{vector: []float32{0.1, 0.2, 0.3}}
	u := newTestUsecaseWithNeighbor(fake)
	u.neighborEncoder = encoder

	direct := []Citation{{ArticleID: uuid.New().String(), Title: "Direct A"}}

	related := u.buildRelatedCitations(context.Background(), direct, "user query")
	require.Len(t, related, 1)
	assert.Equal(t, 1, encoder.calls, "the synthetic neighbor query must be embedded once")
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, fake.lastVector,
		"SearchNeighbors must receive the query vector so the pgvector arm is compiled into the SQL")
}

// A failed embedding is a real degradation (lexical-only neighbors), not a
// normal state: it must be loud rather than an implicit nil vector.
func TestBuildRelatedCitations_EmbeddingFailure_LogsAndDegrades(t *testing.T) {
	var buf bytes.Buffer
	fake := &fakeNeighborSearcher{
		hits: []domain.SearchResult{{ArticleID: uuid.New().String(), Title: "Neighbor"}},
	}
	u := newTestUsecaseWithNeighbor(fake)
	u.neighborEncoder = &fakeNeighborEncoder{err: errors.New("embedder down")}
	u.logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	direct := []Citation{{ArticleID: uuid.New().String(), Title: "Direct A"}}

	related := u.buildRelatedCitations(context.Background(), direct, "user query")
	require.Len(t, related, 1, "lexical arm still runs when the encoder is down")
	assert.Empty(t, fake.lastVector)
	assert.Contains(t, buf.String(), "related_citation_embedding_failed")
	assert.Contains(t, buf.String(), "lexical_only")
}

// Zero neighbors is the silent-failure shape this defect actually took: the
// query succeeded, returned no rows, and the code returned nil without a word.
// It must name why it came back empty.
func TestBuildRelatedCitations_ZeroHits_LogsReason(t *testing.T) {
	var buf bytes.Buffer
	fake := &fakeNeighborSearcher{hits: nil}
	u := newTestUsecaseWithNeighbor(fake)
	u.logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	direct := []Citation{{ArticleID: uuid.New().String(), Title: "Direct A"}}

	related := u.buildRelatedCitations(context.Background(), direct, "user query")
	assert.Nil(t, related)

	logged := buf.String()
	assert.Contains(t, logged, "related_citation_empty")
	assert.Contains(t, logged, "seed_count")
	assert.Contains(t, logged, "vector_arm")
}

// An unwired searcher in production means the DI edge ADR-000928 mandated is
// gone. It must not look like "no neighbors found".
func TestBuildRelatedCitations_NoSearcher_LogsUnwired(t *testing.T) {
	var buf bytes.Buffer
	u := &answerWithRAGUsecase{
		neighborLimit: 3,
		logger:        slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}

	direct := []Citation{{ArticleID: uuid.New().String(), Title: "Direct"}}
	assert.Nil(t, u.buildRelatedCitations(context.Background(), direct, "user query"))
	assert.Contains(t, buf.String(), "related_citation_searcher_unwired")
}

// When the searcher is not wired at all (e.g. tests, opt-out deployments),
// neighbor lookup is a hard no-op — no calls are dispatched and no rows
// are returned. This is the safe default established by the option pattern.
func TestBuildRelatedCitations_NoSearcher_ReturnsNil(t *testing.T) {
	u := &answerWithRAGUsecase{
		neighborSearcher: nil,
		neighborLimit:    3,
		logger:           slog.Default(),
	}

	direct := []Citation{
		{ArticleID: uuid.New().String(), Title: "Direct"},
	}

	related := u.buildRelatedCitations(context.Background(), direct, "user query")
	assert.Nil(t, related)
}
