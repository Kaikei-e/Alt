package knowledge_home_projector

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

// ── reproject determinism ──

func TestProjector_ReprojectIsDeterministic(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID := uuid.New()
	base := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	itemKey := fmt.Sprintf("article:%s", articleID)

	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "ArticleCreated", articleID.String(), base, tenant, user, mustJSON(t, map[string]any{
			"article_id":   articleID.String(),
			"title":        "Rust async runtimes compared",
			"published_at": base.Add(-2 * time.Hour).Format(time.RFC3339),
			"url":          "https://example.com/rust-async",
		})),
		homeEvent(2, "SummaryVersionCreated", articleID.String(), base.Add(time.Minute), tenant, user, mustJSON(t, map[string]any{
			"summary_version_id": uuid.New().String(),
			"article_id":         articleID.String(),
			"summary_text":       "A short summary.",
		})),
		homeEvent(3, "TagSetVersionCreated", articleID.String(), base.Add(2*time.Minute), tenant, user, mustJSON(t, map[string]any{
			"tag_set_version_id": uuid.New().String(),
			"article_id":         articleID.String(),
			"tags":               []string{"rust", "async"},
		})),
		homeEvent(4, "HomeItemOpened", itemKey, base.Add(3*time.Minute), tenant, user, mustJSON(t, map[string]any{"item_key": itemKey})),
		homeEvent(5, "HomeItemDismissed", itemKey, base.Add(4*time.Minute), tenant, user, mustJSON(t, map[string]any{"item_key": itemKey})),
		homeEvent(6, "SummarySuperseded", articleID.String(), base.Add(5*time.Minute), tenant, user, mustJSON(t, map[string]any{
			"article_id":               articleID.String(),
			"new_summary_version_id":   uuid.New().String(),
			"old_summary_version_id":   uuid.New().String(),
			"previous_summary_excerpt": "A short summary.",
		})),
		homeEvent(7, "TagSetSuperseded", articleID.String(), base.Add(6*time.Minute), tenant, user, mustJSON(t, map[string]any{
			"article_id":             articleID.String(),
			"new_tag_set_version_id": uuid.New().String(),
			"old_tag_set_version_id": uuid.New().String(),
			"previous_tags":          []string{"rust", "async"},
		})),
		homeEvent(8, "ReasonMerged", articleID.String(), base.Add(7*time.Minute), tenant, user, mustJSON(t, map[string]any{
			"article_id":         articleID.String(),
			"item_key":           itemKey,
			"added_codes":        []string{"pulse_need_to_know"},
			"previous_why_codes": []string{"new_unread"},
		})),
		homeEvent(9, "ArticleUrlBackfilled", articleID.String(), base.Add(8*time.Minute), tenant, user, mustJSON(t, map[string]any{
			"article_id": articleID.String(),
			"url":        "https://example.com/rust-async-corrected",
		})),
	}

	first := newFakeRepo(events)
	require.NoError(t, NewProjector(first, nil, Config{}).RunBatch(context.Background()))

	second := newFakeRepo(events)
	require.NoError(t, NewProjector(second, nil, Config{}).RunBatch(context.Background()))

	assert.Equal(t, first.homeItems, second.homeItems, "replaying the same event log must reproduce identical knowledge_home_items rows (reproject-safe)")
	assert.Equal(t, first.dismissed, second.dismissed)
	assert.Equal(t, first.digests, second.digests)
	assert.Equal(t, first.recallCandidates, second.recallCandidates)
	assert.Equal(t, first.urlPatches, second.urlPatches)
	assert.Equal(t, first.checkpoint, second.checkpoint)

	// The equality checks above only prove both replays drop added_codes
	// the same way — they would still pass if why_reasons were empty in
	// both. Assert the content directly: event #8's ReasonMerged must have
	// actually reached why_reasons, not just replayed consistently as empty.
	var codes []string
	for _, r := range first.homeItems[itemKey].WhyReasons {
		codes = append(codes, r.Code)
	}
	assert.Contains(t, codes, "pulse_need_to_know", "ReasonMerged's added_codes must reach why_reasons on the folded item")
}

// ── active projection version resolution ──
//
// Regression coverage for the incident where the projector's hardcoded
// currentProjectionVersion=1 diverged from the ACTIVE version (7) in
// knowledge_projection_versions, so every write landed on invisible v1 rows
// and DismissKnowledgeHomeItem's exact-match UPDATE hit 0 rows on v7 data.

func TestProjector_ArticleCreated_UsesActiveProjectionVersionFromRepo(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{
		"article_id": articleID.String(),
		"title":      "Some article",
		"url":        "https://example.com/a",
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "ArticleCreated", articleID.String(), occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	repo.activeProjectionVersion = &sovereign_db.ProjectionVersion{Version: 7}
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	item, ok := repo.homeItems[fmt.Sprintf("article:%s", articleID)]
	require.True(t, ok)
	assert.Equal(t, 7, item.ProjectionVersion, "knowledge_home_items writes must use the ACTIVE projection version, not a hardcoded constant")
}

func TestProjector_HomeItemDismissed_UsesActiveProjectionVersionFromRepo(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	itemKey := "article:" + uuid.New().String()
	occurredAt := time.Date(2026, 7, 18, 23, 30, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{"item_key": itemKey})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "HomeItemDismissed", itemKey, occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	repo.activeProjectionVersion = &sovereign_db.ProjectionVersion{Version: 7}
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	d, ok := repo.dismissed[itemKey]
	require.True(t, ok)
	assert.Equal(t, 7, d.ProjectionVersion, "dismiss writes must use the ACTIVE projection version so the exact-match UPDATE targets live v7 rows, not invisible v1 rows")
}

func TestProjector_RunBatch_ErrorsWhenNoActiveProjectionVersion(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{
		"article_id": articleID.String(),
		"title":      "Some article",
		"url":        "https://example.com/a",
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "ArticleCreated", articleID.String(), occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	repo.activeProjectionVersion = nil // no knowledge_projection_versions row with status='active'
	p := NewProjector(repo, nil, Config{})

	err := p.RunBatch(context.Background())
	require.Error(t, err, "a missing active projection version must fail the batch loudly, never default to version 1")

	assert.Empty(t, repo.homeItems, "no writes must happen when the active version cannot be resolved")
	assert.Equal(t, int64(0), repo.checkpoint, "checkpoint must not advance when the active version lookup fails")
}

func TestProjector_RunBatch_PropagatesActiveProjectionVersionLookupError(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{
		"article_id": articleID.String(),
		"title":      "Some article",
		"url":        "https://example.com/a",
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "ArticleCreated", articleID.String(), occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	repo.activeVersionErr = fmt.Errorf("knowledge_projection_versions unavailable")
	p := NewProjector(repo, nil, Config{})

	err := p.RunBatch(context.Background())
	require.Error(t, err, "an active-version lookup failure must fail the batch loudly")
	assert.Empty(t, repo.homeItems)
}

func TestProjector_RunBatch_SkipsActiveVersionLookupWhenNoEvents(t *testing.T) {
	repo := newFakeRepo(nil)
	repo.activeVersionErr = fmt.Errorf("must not be called when there is nothing to fold")
	p := NewProjector(repo, nil, Config{})

	require.NoError(t, p.RunBatch(context.Background()), "an empty batch must return before resolving the active version")
}

func TestProjector_FoldEventUsesExplicitlyPassedVersionRegardlessOfActive(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	itemKey := "article:" + uuid.New().String()
	occurredAt := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{"item_key": itemKey})
	evt := homeEvent(1, "HomeItemDismissed", itemKey, occurredAt, tenant, user, payload)

	repo := newFakeRepo(nil)
	repo.activeProjectionVersion = &sovereign_db.ProjectionVersion{Version: 7} // active=7, must be ignored below
	p := NewProjector(repo, nil, Config{})

	// A future reproject/backfill caller (e.g. building a shadow v6 while v7
	// stays active for live reads/writes) drives the fold directly with an
	// explicit target version — it must never be silently overridden by the
	// live batch's active-version lookup.
	require.NoError(t, p.foldEvent(context.Background(), evt, 6))

	d, ok := repo.dismissed[itemKey]
	require.True(t, ok)
	assert.Equal(t, 6, d.ProjectionVersion, "an explicitly-passed fold version must win over the repo's active version")
}
