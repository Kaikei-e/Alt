package trail_episodes

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

func fp(itemKey, footprintKey string, occurredAt, firstOccurredAt time.Time, tags []string, wear string) sovereign_db.TrailFootprint {
	return sovereign_db.TrailFootprint{
		FootprintKey:    footprintKey,
		Verb:            "read",
		ItemKey:         itemKey,
		Tags:            tags,
		OccurredAt:      occurredAt,
		FirstOccurredAt: firstOccurredAt,
		ContactCount:    1,
		Wear:            wear,
	}
}

func TestDerive_SameArticleContactsAlwaysJoinAcrossDays(t *testing.T) {
	day1 := time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	footprints := []sovereign_db.TrailFootprint{
		fp("article:a", "fp:a-read", day2, day2, nil, "thin"),
		fp("article:a", "fp:a-asked", day1, day1, nil, "thin"),
	}

	episodes := Derive(footprints)

	require.Len(t, episodes, 1, "all contacts on the same article join one episode unconditionally")
	ep := episodes[0]
	assert.Equal(t, "ep:fp:a-asked", ep.EpisodeKey, "episode key derives from the oldest member")
	require.Len(t, ep.Footprints, 2)
	assert.Equal(t, "fp:a-read", ep.Footprints[0].FootprintKey, "members are newest contact first")
	assert.Equal(t, "fp:a-asked", ep.Footprints[1].FootprintKey)
}

func TestDerive_CrossArticleTagOverlapWithinWindowJoins(t *testing.T) {
	early := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	later := early.Add(5 * 24 * time.Hour) // well within the 14d window

	footprints := []sovereign_db.TrailFootprint{
		fp("article:b", "fp:b", later, later, []string{"Rust", "Async", "extra"}, "thin"),
		fp("article:a", "fp:a", early, early, []string{"rust", "async"}, "thin"),
	}

	episodes := Derive(footprints)

	require.Len(t, episodes, 1, ">=2 cleaned-tag overlap within the window must chain the two articles")
	assert.Equal(t, "ep:fp:a", episodes[0].EpisodeKey)
	assert.Len(t, episodes[0].Footprints, 2)
}

func TestDerive_SharedJunkTagsDoNotJoin(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	t1 := t0.Add(2 * 24 * time.Hour)

	footprints := []sovereign_db.TrailFootprint{
		fp("article:b", "fp:b", t1, t1, []string{"also", "5", "navy"}, "thin"),
		fp("article:a", "fp:a", t0, t0, []string{"also", "5", "military"}, "thin"),
	}

	episodes := Derive(footprints)

	assert.Len(t, episodes, 2, "also/5 normalize to empty and must not count toward the shared-tag threshold")
}

func TestDerive_OutsideWindowDoesNotJoin(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	t1 := t0.Add(20 * 24 * time.Hour) // outside the 14d window

	footprints := []sovereign_db.TrailFootprint{
		fp("article:b", "fp:b", t1, t1, []string{"rust", "async"}, "thin"),
		fp("article:a", "fp:a", t0, t0, []string{"rust", "async"}, "thin"),
	}

	episodes := Derive(footprints)

	assert.Len(t, episodes, 2, "a gap beyond tagChainWindow must keep the articles in separate episodes")
}

func TestDerive_OneSharedTagDoesNotJoin(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	t1 := t0.Add(2 * 24 * time.Hour)

	footprints := []sovereign_db.TrailFootprint{
		fp("article:b", "fp:b", t1, t1, []string{"rust", "postgres"}, "thin"),
		fp("article:a", "fp:a", t0, t0, []string{"rust", "async"}, "thin"),
	}

	episodes := Derive(footprints)

	assert.Len(t, episodes, 2, "a single shared tag is too weak a signal to chain (precision over recall)")
}

func TestDerive_EpisodesOrderedNewestLatestContactFirst(t *testing.T) {
	oldEpisode := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	newEpisode := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

	footprints := []sovereign_db.TrailFootprint{
		fp("article:new", "fp:new", newEpisode, newEpisode, nil, "thin"),
		fp("article:old", "fp:old", oldEpisode, oldEpisode, nil, "thin"),
	}

	episodes := Derive(footprints)

	require.Len(t, episodes, 2)
	assert.Equal(t, "ep:fp:new", episodes[0].EpisodeKey, "newest latest-contact episode sorts first")
	assert.Equal(t, "ep:fp:old", episodes[1].EpisodeKey)
}

func TestDerive_IsDeterministicAcrossRepeatedCalls(t *testing.T) {
	base := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	footprints := []sovereign_db.TrailFootprint{
		fp("article:c", "fp:c", base.Add(10*24*time.Hour), base.Add(10*24*time.Hour), []string{"rust", "async"}, "thin"),
		fp("article:b", "fp:b", base.Add(5*24*time.Hour), base.Add(5*24*time.Hour), []string{"rust", "async"}, "thin"),
		fp("article:a", "fp:a", base, base, []string{"rust", "async"}, "thin"),
	}

	first := Derive(footprints)
	firstKeys := make([]string, len(first))
	for i, ep := range first {
		firstKeys[i] = ep.EpisodeKey
	}

	for i := 0; i < 20; i++ {
		got := Derive(footprints)
		keys := make([]string, len(got))
		for j, ep := range got {
			keys[j] = ep.EpisodeKey
		}
		assert.Equal(t, firstKeys, keys, "derivation must be deterministic on repeated calls (guards map-iteration flakiness)")
	}
}

func TestDerive_WearIsDeepestAmongMembers(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	t1 := t0.Add(2 * 24 * time.Hour)

	footprints := []sovereign_db.TrailFootprint{
		fp("article:a", "fp:a-2", t1, t1, nil, "deep"),
		fp("article:a", "fp:a-1", t0, t0, nil, "thin"),
	}

	episodes := Derive(footprints)

	require.Len(t, episodes, 1)
	assert.Equal(t, "deep", episodes[0].Wear, "wear escalates to the deepest member band")
}

func TestDerive_UnknownWearTreatedAsThin(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

	footprints := []sovereign_db.TrailFootprint{
		fp("article:a", "fp:a", t0, t0, nil, "mystery-band"),
	}

	episodes := Derive(footprints)

	require.Len(t, episodes, 1)
	assert.Equal(t, "thin", episodes[0].Wear, "an unrecognized wear band must not outrank thin")
}

func TestDerive_EmptyInputYieldsNoEpisodes(t *testing.T) {
	assert.Empty(t, Derive(nil))
}
