package search_trail_usecase

import (
	"context"
	"errors"
	"testing"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSearchPort struct {
	hits      []domain.SearchIndexerArticleHit
	err       error
	gotQuery  string
	gotUserID string
	calls     int
}

func (f *fakeSearchPort) SearchArticles(_ context.Context, query string, userID string) ([]domain.SearchIndexerArticleHit, error) {
	f.calls++
	f.gotQuery = query
	f.gotUserID = userID
	return f.hits, f.err
}

func (f *fakeSearchPort) SearchArticlesWithPagination(_ context.Context, _ string, _ string, _ int, _ int) ([]domain.SearchIndexerArticleHit, int64, error) {
	return nil, 0, nil
}

func (f *fakeSearchPort) SearchRecapsByTag(_ context.Context, _ string, _ int) ([]*domain.RecapSearchResult, error) {
	return nil, nil
}

func (f *fakeSearchPort) SearchRecapsByQuery(_ context.Context, _ string, _ int) ([]*domain.RecapSearchResult, int64, error) {
	return nil, 0, nil
}

type fakeSearchTrailPort struct {
	episodes    []domain.TrailEpisode
	err         error
	gotItemKeys []string
	gotLimit    int
	calls       int
}

func (f *fakeSearchTrailPort) SearchTrailFootprints(_ context.Context, _ uuid.UUID, itemKeys []string, limit int) ([]domain.TrailEpisode, error) {
	f.calls++
	f.gotItemKeys = itemKeys
	f.gotLimit = limit
	return f.episodes, f.err
}

type fakeThumbnailPort struct {
	urls  map[string]string
	err   error
	calls int
}

func (f *fakeThumbnailPort) GetOgImageURLsByArticleIDs(_ context.Context, _ []string) (map[string]string, error) {
	f.calls++
	return f.urls, f.err
}

func episodeWithItemKey(episodeKey, itemKey string) domain.TrailEpisode {
	return domain.TrailEpisode{
		EpisodeKey: episodeKey,
		Footprints: []domain.TrailFootprint{{FootprintKey: itemKey, ItemKey: itemKey}},
	}
}

// An empty (or whitespace-only) query is a structurally invalid request —
// mirrors the ErrInvalidRequest convention of resolve_trail_branch_usecase /
// emit_trail_outcome_usecase. Neither collaborator is called.
func TestExecute_EmptyQueryIsInvalidRequest(t *testing.T) {
	for _, q := range []string{"", "   ", "\t\n"} {
		searchPort := &fakeSearchPort{}
		trailPort := &fakeSearchTrailPort{}
		uc := NewSearchTrailUsecase(searchPort, trailPort, &fakeThumbnailPort{})

		_, err := uc.Execute(context.Background(), uuid.New(), q, 20)
		require.Error(t, err, "query %q must be rejected", q)
		assert.ErrorIs(t, err, ErrInvalidRequest)
		assert.Zero(t, searchPort.calls, "an invalid request must not reach the search port")
		assert.Zero(t, trailPort.calls, "an invalid request must not reach the trail port")
	}
}

// When the article search returns zero hits, the usecase must short-circuit
// without calling sovereign at all — there is nothing to narrow by.
func TestExecute_ZeroSearchHitsSkipsSovereignCall(t *testing.T) {
	searchPort := &fakeSearchPort{hits: nil}
	trailPort := &fakeSearchTrailPort{}
	uc := NewSearchTrailUsecase(searchPort, trailPort, &fakeThumbnailPort{})

	res, err := uc.Execute(context.Background(), uuid.New(), "rust async", 20)
	require.NoError(t, err)
	assert.Empty(t, res.Episodes)
	assert.Empty(t, res.MatchedItemKeys)
	assert.Zero(t, trailPort.calls, "zero search hits must not call sovereign's GetTrailFootprints")
}

// Search hits are mapped to "article:<id>" item_keys and passed to the trail
// port's filter, alongside a generously large limit (there is no cursor —
// the usecase itself pages the final result).
func TestExecute_MapsSearchHitsToItemKeysAndNarrowsViaTrailPort(t *testing.T) {
	searchPort := &fakeSearchPort{hits: []domain.SearchIndexerArticleHit{{ID: "a1"}, {ID: "a2"}}}
	trailPort := &fakeSearchTrailPort{episodes: []domain.TrailEpisode{episodeWithItemKey("ep:a1", "article:a1")}}
	uc := NewSearchTrailUsecase(searchPort, trailPort, &fakeThumbnailPort{})

	_, err := uc.Execute(context.Background(), uuid.New(), "llm", 20)
	require.NoError(t, err)
	assert.Equal(t, []string{"article:a1", "article:a2"}, trailPort.gotItemKeys)
	assert.Equal(t, sovereignSearchWindow, trailPort.gotLimit, "the sovereign call uses a large fixed window, not the caller's page limit")
	assert.Equal(t, "llm", searchPort.gotQuery)
}

// matched_item_keys is the subset of searched item_keys that actually appear
// among a member of any RETURNED episode. article:a2 has no episode (e.g. it
// fell outside the sovereign derivation window), so it must not be reported
// as matched even though it was searched.
func TestExecute_MatchedItemKeysOnlyIncludesKeysPresentInReturnedEpisodes(t *testing.T) {
	searchPort := &fakeSearchPort{hits: []domain.SearchIndexerArticleHit{{ID: "a1"}, {ID: "a2"}}}
	trailPort := &fakeSearchTrailPort{episodes: []domain.TrailEpisode{episodeWithItemKey("ep:a1", "article:a1")}}
	uc := NewSearchTrailUsecase(searchPort, trailPort, &fakeThumbnailPort{})

	res, err := uc.Execute(context.Background(), uuid.New(), "llm", 20)
	require.NoError(t, err)
	assert.Equal(t, []string{"article:a1"}, res.MatchedItemKeys)
}

// A non-positive limit falls back to the default page size (mirrors
// get_knowledge_trail_usecase.TestExecute_NormalizesLimit).
func TestExecute_ClampsNonPositiveLimitToDefault(t *testing.T) {
	episodes := make([]domain.TrailEpisode, 25)
	for i := range episodes {
		episodes[i] = episodeWithItemKey("ep:"+string(rune('a'+i)), "article:x")
	}
	searchPort := &fakeSearchPort{hits: []domain.SearchIndexerArticleHit{{ID: "x"}}}
	trailPort := &fakeSearchTrailPort{episodes: episodes}
	uc := NewSearchTrailUsecase(searchPort, trailPort, &fakeThumbnailPort{})

	res, err := uc.Execute(context.Background(), uuid.New(), "llm", 0)
	require.NoError(t, err)
	assert.Len(t, res.Episodes, defaultLimit)
}

// An oversized limit (> maxLimit) falls back to the default page size —
// mirrors get_knowledge_trail_usecase, which resets to the default rather
// than clamping down to the max threshold itself.
func TestExecute_ClampsOversizedLimitToDefault(t *testing.T) {
	episodes := make([]domain.TrailEpisode, 150)
	for i := range episodes {
		episodes[i] = domain.TrailEpisode{EpisodeKey: "ep", Footprints: []domain.TrailFootprint{{ItemKey: "article:x"}}}
	}
	searchPort := &fakeSearchPort{hits: []domain.SearchIndexerArticleHit{{ID: "x"}}}
	trailPort := &fakeSearchTrailPort{episodes: episodes}
	uc := NewSearchTrailUsecase(searchPort, trailPort, &fakeThumbnailPort{})

	res, err := uc.Execute(context.Background(), uuid.New(), "llm", maxLimit+1)
	require.NoError(t, err)
	assert.Len(t, res.Episodes, defaultLimit)
}

func TestExecute_PropagatesSearchPortError(t *testing.T) {
	searchPort := &fakeSearchPort{err: errors.New("search-indexer down")}
	trailPort := &fakeSearchTrailPort{}
	uc := NewSearchTrailUsecase(searchPort, trailPort, &fakeThumbnailPort{})

	_, err := uc.Execute(context.Background(), uuid.New(), "llm", 20)
	require.Error(t, err)
	assert.Zero(t, trailPort.calls)
}

func TestExecute_PropagatesTrailPortError(t *testing.T) {
	searchPort := &fakeSearchPort{hits: []domain.SearchIndexerArticleHit{{ID: "a1"}}}
	trailPort := &fakeSearchTrailPort{err: errors.New("sovereign down")}
	uc := NewSearchTrailUsecase(searchPort, trailPort, &fakeThumbnailPort{})

	_, err := uc.Execute(context.Background(), uuid.New(), "llm", 20)
	require.Error(t, err)
}

// D29: episode thumbnails are enriched the same way get_knowledge_trail_usecase
// does it (shared trail_thumbnail_enrichment package) — proves the wiring,
// the exhaustive edge cases already live in that package's own tests / the
// sibling usecase's tests.
func TestExecute_EnrichesEpisodeThumbnailFromRepresentativeArticle(t *testing.T) {
	articleID := uuid.New().String()
	searchPort := &fakeSearchPort{hits: []domain.SearchIndexerArticleHit{{ID: articleID}}}
	trailPort := &fakeSearchTrailPort{episodes: []domain.TrailEpisode{{
		EpisodeKey: "ep:1",
		Footprints: []domain.TrailFootprint{{FootprintKey: "fp:1", ItemKey: "article:" + articleID}},
	}}}
	thumbs := &fakeThumbnailPort{urls: map[string]string{articleID: "https://example.com/a.png"}}
	uc := NewSearchTrailUsecase(searchPort, trailPort, thumbs)

	res, err := uc.Execute(context.Background(), uuid.New(), "llm", 20)
	require.NoError(t, err)
	require.Len(t, res.Episodes, 1)
	assert.Equal(t, "https://example.com/a.png", res.Episodes[0].ThumbnailURL)
	assert.Equal(t, 1, thumbs.calls)
}
