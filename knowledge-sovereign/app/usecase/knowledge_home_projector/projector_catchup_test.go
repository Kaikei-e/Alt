package knowledge_home_projector

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

func TestNewProjector_PanicsWhenRepositoryIsNil(t *testing.T) {
	assert.Panics(t, func() {
		NewProjector(nil, nil, Config{})
	}, "Rule 8: nil repository must panic at construction instead of deferring a nil-deref")
}

func TestRunBatch_CrossesBurnedHolesWithinOneTick(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	t0 := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)

	const numHoles = 15
	const runLen = 2

	var events []sovereign_db.KnowledgeEvent
	var expectedKeys []string
	seq := int64(1)
	for r := 0; r <= numHoles; r++ {
		for j := 0; j < runLen; j++ {
			articleID := uuid.New()
			events = append(events, homeEvent(seq, "ArticleCreated", articleID.String(), t0.Add(time.Duration(seq)*time.Second), tenant, user, mustJSON(t, map[string]any{
				"article_id": articleID.String(),
				"title":      fmt.Sprintf("Article %d", seq),
				"url":        fmt.Sprintf("https://example.test/%d", seq),
			})))
			expectedKeys = append(expectedKeys, "article:"+articleID.String())
			seq++
		}
		if r < numHoles {
			// Skip one sequence to represent a burned hole.
			seq++
		}
	}
	headSeq := events[len(events)-1].EventSeq

	repo := newFakeRepo(events)
	repo.autoBurnedFrontiers = true
	p := NewProjector(repo, nil, Config{})

	require.NoError(t, p.RunBatch(context.Background()))

	assert.Equal(t, headSeq, repo.checkpoint, "burned holes with no in-flight tx must be crossed in the same tick")
	assert.Len(t, repo.homeItems, len(expectedKeys))
	for _, k := range expectedKeys {
		assert.Contains(t, repo.homeItems, k)
	}
	assert.Len(t, repo.frontierReads, numHoles*2, "expected two frontier reads per burned hole")
	for i := 1; i < len(repo.frontierReads); i++ {
		assert.Greater(t, repo.frontierReads[i].Ceiling, repo.frontierReads[i-1].Ceiling)
	}
}

func TestRunBatch_ManyBurnedHolesNeedBoundedTicks(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	t0 := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)

	const numHoles = 200
	events := make([]sovereign_db.KnowledgeEvent, 0, numHoles+1)
	for i := 0; i <= numHoles; i++ {
		seq := int64(i*2 + 1)
		events = append(events, homeEvent(seq, "SomeFutureEventType", fmt.Sprintf("evt-%d", seq), t0, tenant, user, mustJSON(t, map[string]any{})))
	}
	headSeq := events[len(events)-1].EventSeq

	repo := newFakeRepo(events)
	repo.autoBurnedFrontiers = true
	p := NewProjector(repo, nil, Config{})

	calls := 0
	const safetyCap = 500
	for repo.checkpoint < headSeq && calls < safetyCap {
		require.NoError(t, p.RunBatch(context.Background()))
		calls++
	}

	require.Equal(t, headSeq, repo.checkpoint)
	// With defaultMaxTick == 40, each tick handles ~20 (fold + abandon) pairs, so 200 holes takes <= 15 ticks.
	assert.LessOrEqual(t, calls, 15, "default MaxBatchesPerTick must drain burned holes in bounded ticks")
}

func TestRunBatch_HoleFilledBeforeSecondReadIsNotAbandoned(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	t0 := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
	first := uuid.New()
	second := uuid.New()

	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		homeEvent(2, "ArticleCreated", second.String(), t0, tenant, user, mustJSON(t, map[string]any{
			"article_id": second.String(),
			"title":      "Second",
			"url":        "https://example.test/second",
		})),
	})
	repo.autoBurnedFrontiers = true
	probeCount := 0
	repo.beforeGapFrontier = func() {
		probeCount++
		if probeCount == 2 {
			repo.events = append([]sovereign_db.KnowledgeEvent{
				homeEvent(1, "ArticleCreated", first.String(), t0, tenant, user, mustJSON(t, map[string]any{
					"article_id": first.String(),
					"title":      "First",
					"url":        "https://example.test/first",
				})),
			}, repo.events...)
		}
	}

	p := NewProjector(repo, nil, Config{})

	require.NoError(t, p.RunBatch(context.Background()))
	assert.Zero(t, repo.checkpoint, "must not advance past the hole when the second same-tick read sees it filled")
	assert.Len(t, repo.frontierReads, 2)
	assert.True(t, repo.frontierReads[0].HoleOpen)
	assert.False(t, repo.frontierReads[1].HoleOpen)
	assert.Empty(t, repo.homeItems)

	require.NoError(t, p.RunBatch(context.Background()))
	assert.EqualValues(t, 2, repo.checkpoint)
	assert.Contains(t, repo.homeItems, "article:"+first.String())
	assert.Contains(t, repo.homeItems, "article:"+second.String())
}

func TestRunBatch_AdvanceRejectedWhileCrossingHolesEndsTheTick(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	t0 := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)

	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		homeEvent(1, "SomeFutureEventType", "a", t0, tenant, user, mustJSON(t, map[string]any{})),
		// seq 2 is burned
		homeEvent(3, "SomeFutureEventType", "b", t0, tenant, user, mustJSON(t, map[string]any{})),
		// seq 4 is burned
		homeEvent(5, "SomeFutureEventType", "c", t0, tenant, user, mustJSON(t, map[string]any{})),
	})
	repo.autoBurnedFrontiers = true

	rejectedOnce := false
	repo.beforeAdvance = func(_ sovereign_db.ProjectionCheckpoint, toSeq int64) bool {
		if !rejectedOnce && toSeq == 4 {
			rejectedOnce = true
			return false
		}
		return true
	}

	logs := &recordingHandler{}
	p := NewProjector(repo, slog.New(logs), Config{})

	require.NoError(t, p.RunBatch(context.Background()))
	assert.True(t, rejectedOnce, "expected CAS rejection on middle burned hole seq 4")
	assert.EqualValues(t, 3, repo.checkpoint)
	rec, ok := logs.find("knowledge_home_projector.checkpoint_advance_rejected")
	require.True(t, ok)
	assert.Equal(t, slog.LevelError, rec.Level)
}

func TestFakeRepo_CASConformsToFullTokenSemantics(t *testing.T) {
	ctx := context.Background()

	t.Run("first-boot absent uncontested advances cleanly", func(t *testing.T) {
		repo := newFakeRepo(nil)
		repo.checkpointExists = false
		from, err := repo.ReadProjectionCheckpointForAdvance(ctx, projectorName)
		require.NoError(t, err)
		require.False(t, from.Exists)

		applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, projectorName, from, 900)
		require.NoError(t, err)
		assert.True(t, applied)
		assert.True(t, repo.checkpointExists)
		assert.EqualValues(t, 900, repo.checkpoint)
	})

	t.Run("first-boot absent rejected after concurrent rebuild", func(t *testing.T) {
		repo := newFakeRepo(nil)
		repo.checkpointExists = false
		from, err := repo.ReadProjectionCheckpointForAdvance(ctx, projectorName)
		require.NoError(t, err)
		require.False(t, from.Exists)

		// Concurrent rebuild inserts Present(0, t1).
		repo.checkpoint = 0
		repo.checkpointAt = fakeCheckpointAt.Add(time.Minute)
		repo.checkpointExists = true

		applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, projectorName, from, 900)
		require.NoError(t, err)
		assert.False(t, applied)
		assert.Zero(t, repo.checkpoint)
	})

	t.Run("double rebuild 0 -> 0 ABA rejected by UpdatedAt witness", func(t *testing.T) {
		repo := newFakeRepo(nil)
		repo.checkpoint = 0
		repo.checkpointExists = true
		from, err := repo.ReadProjectionCheckpointForAdvance(ctx, projectorName)
		require.NoError(t, err)

		// Second rebuild resets 0 -> 0 with newer updated_at.
		repo.checkpointAt = repo.checkpointAt.Add(time.Minute)

		applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, projectorName, from, 900)
		require.NoError(t, err)
		assert.False(t, applied)
		assert.Zero(t, repo.checkpoint)
	})
}
