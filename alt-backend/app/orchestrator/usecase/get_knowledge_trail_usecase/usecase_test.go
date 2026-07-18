package get_knowledge_trail_usecase

import (
	"context"
	"errors"
	"testing"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTrailPort struct {
	footprints []domain.TrailFootprint
	branches   []domain.TrailBranch
	episodes   []domain.TrailEpisode
	nextCursor string
	hasMore    bool
	err        error
	gotLimit   int
	gotCursor  string
	gotTags    []string
}

func (f *fakeTrailPort) GetTrailFootprints(_ context.Context, _ uuid.UUID, cursor string, limit int, filterTags []string) ([]domain.TrailFootprint, []domain.TrailBranch, []domain.TrailEpisode, string, bool, error) {
	f.gotCursor = cursor
	f.gotLimit = limit
	f.gotTags = filterTags
	return f.footprints, f.branches, f.episodes, f.nextCursor, f.hasMore, f.err
}

type fakeThumbnailPort struct {
	urls   map[string]string
	err    error
	gotIDs []string
	calls  int
}

func (f *fakeThumbnailPort) GetOgImageURLsByArticleIDs(_ context.Context, articleIDs []string) (map[string]string, error) {
	f.calls++
	f.gotIDs = articleIDs
	return f.urls, f.err
}

func TestExecute_ReturnsFootprints(t *testing.T) {
	port := &fakeTrailPort{
		footprints: []domain.TrailFootprint{{FootprintKey: "open:a", Verb: "read"}},
		nextCursor: "cur",
		hasMore:    true,
	}
	uc := NewGetKnowledgeTrailUsecase(port, &fakeThumbnailPort{})

	res, err := uc.Execute(context.Background(), uuid.New(), "", 20, []string{"rust", "async"})
	require.NoError(t, err)
	assert.Len(t, res.Footprints, 1)
	assert.Equal(t, "cur", res.NextCursor)
	assert.True(t, res.HasMore)
	assert.Equal(t, []string{"rust", "async"}, port.gotTags, "theme-lens filter tags pass through to the port")
}

func TestExecute_NormalizesLimit(t *testing.T) {
	port := &fakeTrailPort{}
	uc := NewGetKnowledgeTrailUsecase(port, &fakeThumbnailPort{})

	_, err := uc.Execute(context.Background(), uuid.New(), "", 0, nil)
	require.NoError(t, err)
	assert.Equal(t, defaultLimit, port.gotLimit, "non-positive limit falls back to the default")

	_, err = uc.Execute(context.Background(), uuid.New(), "", 9999, nil)
	require.NoError(t, err)
	assert.Equal(t, defaultLimit, port.gotLimit, "oversized limit falls back to the default")
}

func TestExecute_PropagatesError(t *testing.T) {
	port := &fakeTrailPort{err: errors.New("sovereign down")}
	uc := NewGetKnowledgeTrailUsecase(port, &fakeThumbnailPort{})

	_, err := uc.Execute(context.Background(), uuid.New(), "", 20, nil)
	require.Error(t, err)
}

// Episodes (D24/D30, Wave 8) flow through the usecase alongside the legacy
// footprints/branches.
func TestExecute_EpisodesFlowThrough(t *testing.T) {
	port := &fakeTrailPort{
		episodes: []domain.TrailEpisode{{
			EpisodeKey: "ep:open:article:1",
			Wear:       "worn",
			Footprints: []domain.TrailFootprint{{FootprintKey: "open:article:1", ItemKey: "article:1"}},
		}},
	}
	uc := NewGetKnowledgeTrailUsecase(port, &fakeThumbnailPort{})

	res, err := uc.Execute(context.Background(), uuid.New(), "", 20, nil)
	require.NoError(t, err)
	require.Len(t, res.Episodes, 1)
	assert.Equal(t, "ep:open:article:1", res.Episodes[0].EpisodeKey)
	assert.Equal(t, "worn", res.Episodes[0].Wear)
}

// D29: the representative (first/newest) member's article id is looked up
// for the episode's thumbnail.
func TestExecute_EnrichesEpisodeThumbnailFromRepresentativeArticle(t *testing.T) {
	articleID := uuid.New().String()
	port := &fakeTrailPort{
		episodes: []domain.TrailEpisode{{
			EpisodeKey: "ep:open:article:1",
			Footprints: []domain.TrailFootprint{
				{FootprintKey: "open:article:1", ItemKey: "article:" + articleID},
			},
		}},
	}
	thumbs := &fakeThumbnailPort{urls: map[string]string{articleID: "https://example.com/a.png"}}
	uc := NewGetKnowledgeTrailUsecase(port, thumbs)

	res, err := uc.Execute(context.Background(), uuid.New(), "", 20, nil)
	require.NoError(t, err)
	require.Len(t, res.Episodes, 1)
	assert.Equal(t, "https://example.com/a.png", res.Episodes[0].ThumbnailURL)
	assert.Equal(t, []string{articleID}, thumbs.gotIDs)
}

// A lookup miss (no article_heads row yet) must not fabricate a thumbnail.
func TestExecute_ThumbnailMissLeavesEmpty(t *testing.T) {
	articleID := uuid.New().String()
	port := &fakeTrailPort{
		episodes: []domain.TrailEpisode{{
			EpisodeKey: "ep:open:article:1",
			Footprints: []domain.TrailFootprint{{FootprintKey: "open:article:1", ItemKey: "article:" + articleID}},
		}},
	}
	uc := NewGetKnowledgeTrailUsecase(port, &fakeThumbnailPort{urls: map[string]string{}})

	res, err := uc.Execute(context.Background(), uuid.New(), "", 20, nil)
	require.NoError(t, err)
	require.Len(t, res.Episodes, 1)
	assert.Empty(t, res.Episodes[0].ThumbnailURL)
}

// D29: on a lookup failure, the request degrades to text (no thumbnail)
// rather than failing the whole trail read.
func TestExecute_ThumbnailLookupErrorDegradesRatherThanFails(t *testing.T) {
	articleID := uuid.New().String()
	port := &fakeTrailPort{
		episodes: []domain.TrailEpisode{{
			EpisodeKey: "ep:open:article:1",
			Footprints: []domain.TrailFootprint{{FootprintKey: "open:article:1", ItemKey: "article:" + articleID}},
		}},
	}
	uc := NewGetKnowledgeTrailUsecase(port, &fakeThumbnailPort{err: errors.New("db down")})

	res, err := uc.Execute(context.Background(), uuid.New(), "", 20, nil)
	require.NoError(t, err, "a thumbnail lookup failure must degrade to text, not fail the trail read")
	require.Len(t, res.Episodes, 1)
	assert.Empty(t, res.Episodes[0].ThumbnailURL)
}

// A representative whose item_key isn't an article (no "article:" prefix)
// is skipped rather than looked up.
func TestExecute_NonArticleRepresentativeSkipsThumbnailLookup(t *testing.T) {
	port := &fakeTrailPort{
		episodes: []domain.TrailEpisode{{
			EpisodeKey: "ep:open:podcast:1",
			Footprints: []domain.TrailFootprint{{FootprintKey: "open:podcast:1", ItemKey: "podcast:1"}},
		}},
	}
	thumbs := &fakeThumbnailPort{urls: map[string]string{}}
	uc := NewGetKnowledgeTrailUsecase(port, thumbs)

	res, err := uc.Execute(context.Background(), uuid.New(), "", 20, nil)
	require.NoError(t, err)
	require.Len(t, res.Episodes, 1)
	assert.Empty(t, res.Episodes[0].ThumbnailURL)
	assert.Empty(t, thumbs.gotIDs, "a non-article representative must not trigger a lookup")
}

// No episodes at all must not call the thumbnail port (nothing to enrich).
func TestExecute_NoEpisodesSkipsThumbnailLookupEntirely(t *testing.T) {
	port := &fakeTrailPort{}
	thumbs := &fakeThumbnailPort{}
	uc := NewGetKnowledgeTrailUsecase(port, thumbs)

	_, err := uc.Execute(context.Background(), uuid.New(), "", 20, nil)
	require.NoError(t, err)
	assert.Zero(t, thumbs.calls)
}
