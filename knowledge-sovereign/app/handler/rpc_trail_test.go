package handler

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// The trail read path now serves the derived episode spine (D24/D30/Wave 8):
// episodes populated, legacy footprints superseded (always empty).
func TestGetTrailFootprints_EpisodesPopulatedLegacyFootprintsEmpty(t *testing.T) {
	repo := &mockRepo{trailFootprints: []sovereign_db.TrailFootprint{{
		FootprintKey: "open:article:1",
		Verb:         "read",
		ItemKey:      "article:1",
		Title:        "US military courts in the UK",
		OccurredAt:   time.Date(2026, 7, 7, 22, 20, 0, 0, time.UTC),
		Wear:         "worn",
	}}}
	h := NewSovereignHandler(repo)

	resp, err := h.GetTrailFootprints(context.Background(), connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
		UserId: "22222222-2222-2222-2222-222222222222",
		Limit:  20,
	}))
	require.NoError(t, err)

	assert.Empty(t, resp.Msg.Footprints, "legacy footprints field is superseded by episodes and must stay empty")
	require.Len(t, resp.Msg.Episodes, 1)
	assert.Equal(t, "ep:open:article:1", resp.Msg.Episodes[0].EpisodeKey)
	assert.Equal(t, "worn", resp.Msg.Episodes[0].Wear)
}

// Collapsed contacts (D24) and cleaned tags (D25) surface through the
// episode's member footprints — the same enrichment the legacy field used to
// carry directly.
func TestGetTrailFootprints_CollapsedContactsAndCleanedTagsInsideEpisodeMembers(t *testing.T) {
	first := time.Date(2026, 6, 27, 18, 37, 0, 0, time.UTC)
	latest := time.Date(2026, 7, 7, 22, 20, 0, 0, time.UTC)
	repo := &mockRepo{trailFootprints: []sovereign_db.TrailFootprint{{
		FootprintKey:    "open:article:1",
		Verb:            "read",
		ItemKey:         "article:1",
		Title:           "US military courts in the UK",
		Tags:            []string{"Military", "also", "5", "agents", "agent", "military"},
		OccurredAt:      latest,
		FirstOccurredAt: first,
		ContactCount:    2,
		Wear:            "worn",
	}}}
	h := NewSovereignHandler(repo)

	resp, err := h.GetTrailFootprints(context.Background(), connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
		UserId: "22222222-2222-2222-2222-222222222222",
		Limit:  20,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Episodes, 1)
	require.Len(t, resp.Msg.Episodes[0].Footprints, 1)

	fp := resp.Msg.Episodes[0].Footprints[0]
	assert.Equal(t, int32(2), fp.ContactCount)
	require.NotNil(t, fp.FirstOccurredAt, "the earliest collapsed contact must be mapped")
	assert.Equal(t, first, fp.FirstOccurredAt.AsTime())
	assert.Equal(t, latest, fp.OccurredAt.AsTime())
	assert.Equal(t, []string{"military", "agent"}, fp.Tags,
		"tags must be cleaned (stopwords/digits dropped, case and plural variants merged)")
}

// The handler always fetches a fixed derivation window from the read model,
// regardless of the client's requested page cursor/limit — episodes are
// derived over the window, then paged in the handler.
func TestGetTrailFootprints_FetchesFixedDerivationWindowRegardlessOfClientPaging(t *testing.T) {
	repo := &mockRepo{trailFootprints: []sovereign_db.TrailFootprint{{
		FootprintKey: "open:article:1", ItemKey: "article:1", Verb: "read",
		OccurredAt: time.Now(), Wear: "thin",
	}}}
	h := NewSovereignHandler(repo)

	_, err := h.GetTrailFootprints(context.Background(), connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
		UserId: "22222222-2222-2222-2222-222222222222",
		Cursor: "ep:1",
		Limit:  1,
	}))
	require.NoError(t, err)

	assert.Equal(t, "", repo.gotTrailCursor, "the read-model fetch must always start at the beginning of the window")
	assert.Equal(t, episodeWindowRows, repo.gotTrailLimit, "the read-model fetch must request the full derivation window")
}

// Episode paging: three unrelated articles (distinct tags, far apart in
// time) derive to three separate episodes; the handler pages over them with
// an "ep:<offset>" cursor.
func TestGetTrailFootprints_EpisodePaging(t *testing.T) {
	base := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	repo := &mockRepo{trailFootprints: []sovereign_db.TrailFootprint{
		{FootprintKey: "fp:c", ItemKey: "article:c", Verb: "read", OccurredAt: base.Add(60 * 24 * time.Hour), Tags: []string{"golang"}, Wear: "thin"},
		{FootprintKey: "fp:b", ItemKey: "article:b", Verb: "read", OccurredAt: base.Add(30 * 24 * time.Hour), Tags: []string{"postgres"}, Wear: "thin"},
		{FootprintKey: "fp:a", ItemKey: "article:a", Verb: "read", OccurredAt: base, Tags: []string{"rust"}, Wear: "thin"},
	}}
	h := NewSovereignHandler(repo)

	page1, err := h.GetTrailFootprints(context.Background(), connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
		UserId: "22222222-2222-2222-2222-222222222222",
		Limit:  1,
	}))
	require.NoError(t, err)
	require.Len(t, page1.Msg.Episodes, 1)
	assert.Equal(t, "ep:fp:c", page1.Msg.Episodes[0].EpisodeKey)
	assert.True(t, page1.Msg.HasMore)
	assert.Equal(t, "ep:1", page1.Msg.NextCursor)

	page2, err := h.GetTrailFootprints(context.Background(), connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
		UserId: "22222222-2222-2222-2222-222222222222",
		Cursor: page1.Msg.NextCursor,
		Limit:  1,
	}))
	require.NoError(t, err)
	require.Len(t, page2.Msg.Episodes, 1)
	assert.Equal(t, "ep:fp:b", page2.Msg.Episodes[0].EpisodeKey)
	assert.True(t, page2.Msg.HasMore)
	assert.Equal(t, "ep:2", page2.Msg.NextCursor)

	page3, err := h.GetTrailFootprints(context.Background(), connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
		UserId: "22222222-2222-2222-2222-222222222222",
		Cursor: page2.Msg.NextCursor,
		Limit:  1,
	}))
	require.NoError(t, err)
	require.Len(t, page3.Msg.Episodes, 1)
	assert.Equal(t, "ep:fp:a", page3.Msg.Episodes[0].EpisodeKey)
	assert.False(t, page3.Msg.HasMore)
	assert.Empty(t, page3.Msg.NextCursor)
}

// FilterItemKeys (Wave 9 — trail search, D25) narrows the derived episodes to
// those containing at least one matching member, but a matching episode
// surfaces in FULL: a member that did not itself match the filter still comes
// along, because episodes (not footprints) are the unit of context.
func TestGetTrailFootprints_FilterItemKeysNarrowsToMatchingEpisodesWithFullMemberContext(t *testing.T) {
	base := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	repo := &mockRepo{trailFootprints: []sovereign_db.TrailFootprint{
		// article:a and article:b chain into one episode (stage 2: shared
		// tags within the window).
		{FootprintKey: "fp:b", ItemKey: "article:b", Verb: "read", OccurredAt: base.Add(24 * time.Hour), Tags: []string{"golang", "backend"}, Wear: "thin"},
		{FootprintKey: "fp:a", ItemKey: "article:a", Verb: "read", OccurredAt: base, Tags: []string{"golang", "backend"}, Wear: "thin"},
		// article:z is unrelated (different tags, far away in time) and forms
		// its own episode.
		{FootprintKey: "fp:z", ItemKey: "article:z", Verb: "read", OccurredAt: base.Add(100 * 24 * time.Hour), Tags: []string{"rust"}, Wear: "thin"},
	}}
	h := NewSovereignHandler(repo)

	resp, err := h.GetTrailFootprints(context.Background(), connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
		UserId:         "22222222-2222-2222-2222-222222222222",
		Limit:          20,
		FilterItemKeys: []string{"article:a"},
	}))
	require.NoError(t, err)

	require.Len(t, resp.Msg.Episodes, 1, "only the episode containing article:a should surface")
	members := resp.Msg.Episodes[0].Footprints
	require.Len(t, members, 2, "the whole episode comes along, including the article:b member that did not match the filter")
	gotItemKeys := []string{members[0].ItemKey, members[1].ItemKey}
	assert.ElementsMatch(t, []string{"article:a", "article:b"}, gotItemKeys)
}

// An empty FilterItemKeys must leave episode derivation and paging unchanged
// (the zero value must not be misread as "match nothing").
func TestGetTrailFootprints_EmptyFilterItemKeysReturnsAllEpisodesUnchanged(t *testing.T) {
	base := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	repo := &mockRepo{trailFootprints: []sovereign_db.TrailFootprint{
		{FootprintKey: "fp:b", ItemKey: "article:b", Verb: "read", OccurredAt: base.Add(24 * time.Hour), Tags: []string{"golang", "backend"}, Wear: "thin"},
		{FootprintKey: "fp:a", ItemKey: "article:a", Verb: "read", OccurredAt: base, Tags: []string{"golang", "backend"}, Wear: "thin"},
		{FootprintKey: "fp:z", ItemKey: "article:z", Verb: "read", OccurredAt: base.Add(100 * 24 * time.Hour), Tags: []string{"rust"}, Wear: "thin"},
	}}
	h := NewSovereignHandler(repo)

	resp, err := h.GetTrailFootprints(context.Background(), connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
		UserId: "22222222-2222-2222-2222-222222222222",
		Limit:  20,
	}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Episodes, 2, "no filter_item_keys means both derived episodes come back unchanged")
}

// Paging (limit/cursor) applies AFTER filtering: the offset walks the
// filtered episode list, not the full unfiltered derivation.
func TestGetTrailFootprints_FilterItemKeysPagingAppliesAfterFiltering(t *testing.T) {
	base := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	repo := &mockRepo{trailFootprints: []sovereign_db.TrailFootprint{
		{FootprintKey: "fp:c", ItemKey: "article:c", Verb: "read", OccurredAt: base.Add(60 * 24 * time.Hour), Tags: []string{"golang"}, Wear: "thin"},
		{FootprintKey: "fp:b", ItemKey: "article:b", Verb: "read", OccurredAt: base.Add(30 * 24 * time.Hour), Tags: []string{"postgres"}, Wear: "thin"},
		{FootprintKey: "fp:a", ItemKey: "article:a", Verb: "read", OccurredAt: base, Tags: []string{"rust"}, Wear: "thin"},
	}}
	h := NewSovereignHandler(repo)

	// article:b is excluded from the filter; article:c and article:a match.
	page1, err := h.GetTrailFootprints(context.Background(), connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
		UserId:         "22222222-2222-2222-2222-222222222222",
		Limit:          1,
		FilterItemKeys: []string{"article:c", "article:a"},
	}))
	require.NoError(t, err)
	require.Len(t, page1.Msg.Episodes, 1, "limit applies after filtering down to the matching episodes")
	assert.Equal(t, "ep:fp:c", page1.Msg.Episodes[0].EpisodeKey)
	assert.True(t, page1.Msg.HasMore, "one more matching episode (article:a) remains beyond the limit")
	assert.Equal(t, "ep:1", page1.Msg.NextCursor)

	page2, err := h.GetTrailFootprints(context.Background(), connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
		UserId:         "22222222-2222-2222-2222-222222222222",
		Limit:          1,
		Cursor:         page1.Msg.NextCursor,
		FilterItemKeys: []string{"article:c", "article:a"},
	}))
	require.NoError(t, err)
	require.Len(t, page2.Msg.Episodes, 1)
	assert.Equal(t, "ep:fp:a", page2.Msg.Episodes[0].EpisodeKey, "the excluded article:b episode must not consume a page slot")
	assert.False(t, page2.Msg.HasMore)
}

func TestGetTrailFootprints_InvalidEpisodeCursorIsInvalidArgument(t *testing.T) {
	repo := &mockRepo{}
	h := NewSovereignHandler(repo)

	_, err := h.GetTrailFootprints(context.Background(), connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
		UserId: "22222222-2222-2222-2222-222222222222",
		Cursor: "not-a-valid-cursor",
		Limit:  20,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
