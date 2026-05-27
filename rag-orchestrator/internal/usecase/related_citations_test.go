package usecase

import (
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
	lastSeeds []string
	lastQuery string
	hits      []domain.SearchResult
	err       error
}

func (f *fakeNeighborSearcher) HybridSearch(_ context.Context, _ []float32, _ string, _ int) ([]domain.SearchResult, error) {
	return nil, nil
}

func (f *fakeNeighborSearcher) SearchNeighbors(_ context.Context, _ []float32, queryText string, seeds []string, _ int) ([]domain.SearchResult, error) {
	f.lastSeeds = append([]string(nil), seeds...)
	f.lastQuery = queryText
	if f.err != nil {
		return nil, f.err
	}
	return f.hits, nil
}

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
