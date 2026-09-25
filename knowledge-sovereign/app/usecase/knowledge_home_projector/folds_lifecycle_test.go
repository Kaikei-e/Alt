package knowledge_home_projector

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

// ── HomeItemOpened ──

func TestProjector_FoldsHomeItemOpened(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	itemKey := "article:" + uuid.New().String()
	occurredAt := time.Date(2026, 7, 14, 13, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{"item_key": itemKey})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "HomeItemOpened", itemKey, occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	item, ok := repo.homeItems[itemKey]
	require.True(t, ok)
	assert.Equal(t, 0.1, item.Score, "opening an item suppresses its score")
	assert.Equal(t, "set", item.ScoreOp,
		"suppression must be authoritative (score_op=set) — under the blanket GREATEST merge repository.go used to "+
			"apply, a 0.1 suppressed score could never overwrite a higher stored score and the suppression was unreachable")
	require.NotNil(t, item.LastInteractedAt)
	assert.True(t, occurredAt.Equal(*item.LastInteractedAt))

	assert.Equal(t, 1, repo.clearedSupersede[itemKey], "opening an item clears its supersede state (acknowledgement)")

	cand, ok := repo.recallCandidates[itemKey]
	require.True(t, ok, "opening an item creates a recall candidate")
	require.NotNil(t, cand.FirstEligibleAt)
	assert.True(t, occurredAt.Add(1*time.Hour).Equal(*cand.FirstEligibleAt), "recall eligibility is event-time + 1h, not wall-clock")
}

func TestProjector_HomeItemOpened_ClearSupersedeFailureIsNonFatal(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	itemKey := "article:" + uuid.New().String()
	occurredAt := time.Date(2026, 7, 14, 13, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{"item_key": itemKey})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "HomeItemOpened", itemKey, occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	repo.clearSupersedeErr = fmt.Errorf("clear supersede unavailable")
	p := NewProjector(repo, nil, Config{})

	require.NoError(t, p.RunBatch(context.Background()), "a clear_supersede failure must not fail the batch")
	_, ok := repo.homeItems[itemKey]
	assert.True(t, ok, "the home item upsert must still succeed")
	assert.Equal(t, int64(1), repo.checkpoint)
}

func TestProjector_HomeItemOpened_RecallCandidateFailureIsNonFatal(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	itemKey := "article:" + uuid.New().String()
	occurredAt := time.Date(2026, 7, 14, 13, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{"item_key": itemKey})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "HomeItemOpened", itemKey, occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	repo.recallCandErr = fmt.Errorf("recall_candidate_view unavailable")
	p := NewProjector(repo, nil, Config{})

	require.NoError(t, p.RunBatch(context.Background()), "a recall_candidate upsert failure must not fail the batch")
	_, ok := repo.homeItems[itemKey]
	assert.True(t, ok, "the home item upsert must still succeed")
	assert.Equal(t, int64(1), repo.checkpoint)
}

// ── HomeItemDismissed ──

func TestProjector_FoldsHomeItemDismissed(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	itemKey := "article:" + uuid.New().String()
	occurredAt := time.Date(2026, 7, 14, 14, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{"item_key": itemKey})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "HomeItemDismissed", itemKey, occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	d, ok := repo.dismissed[itemKey]
	require.True(t, ok)
	parsed, err := time.Parse(time.RFC3339Nano, d.DismissedAt)
	require.NoError(t, err)
	assert.True(t, occurredAt.Equal(parsed), "dismissed_at must be the event's own OccurredAt, never a wall-clock fallback")
}

func TestProjector_HomeItemDismissed_FallsBackToAggregateIDWhenPayloadItemKeyEmpty(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	itemKey := "article:" + uuid.New().String()
	occurredAt := time.Date(2026, 7, 14, 14, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{"item_key": ""})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "HomeItemDismissed", itemKey, occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	_, ok := repo.dismissed[itemKey]
	assert.True(t, ok, "an empty payload.item_key must fall back to event.AggregateID")
}

// A client may dismiss an item_key that never produced a knowledge_home_items
// row (the handler only checks item_key is non-empty, and the event is
// appended independently of the write-through). ADR-000473 declared that
// condition non-fatal by design — alt-backend's write-through already logs and
// swallows it. Folding it as a hard failure instead turns one such event into
// a poison pill: the batch stops, the checkpoint never advances past it, and
// every user's Knowledge Home freezes on the same event tick after tick.
func TestProjector_HomeItemDismissed_MissingTargetRowIsBenignAndBatchContinues(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	orphanKey := "article:" + uuid.New().String()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 14, 30, 0, 0, time.UTC)

	dismissPayload := mustJSON(t, map[string]any{"item_key": orphanKey})
	articlePayload := mustJSON(t, map[string]any{
		"article_id": articleID.String(),
		"title":      "Still projected after the orphan dismiss",
		"url":        "https://example.com/after",
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "HomeItemDismissed", orphanKey, occurredAt, tenant, user, dismissPayload),
		homeEvent(2, "ArticleCreated", articleID.String(), occurredAt.Add(time.Minute), tenant, user, articlePayload),
	}
	repo := newFakeRepo(events)
	repo.dismissMissingKeys[orphanKey] = true
	p := NewProjector(repo, nil, Config{})

	require.NoError(t, p.RunBatch(context.Background()), "a dismiss whose target row does not exist must not fail the batch")
	assert.Equal(t, int64(2), repo.checkpoint, "checkpoint must advance past the orphan dismiss, not wedge on it forever")

	_, ok := repo.homeItems[fmt.Sprintf("article:%s", articleID)]
	assert.True(t, ok, "events after the orphan dismiss must still project")
}

func TestProjector_HomeItemDismissed_UnexpectedRepositoryFailureStopsBatch(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	itemKey := "article:" + uuid.New().String()
	occurredAt := time.Date(2026, 7, 14, 14, 45, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{"item_key": itemKey})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "HomeItemDismissed", itemKey, occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	repo.dismissHomeErr = fmt.Errorf("knowledge_home_items unavailable")
	p := NewProjector(repo, nil, Config{})

	err := p.RunBatch(context.Background())
	require.Error(t, err, "only the not-found condition is benign — a genuine repository failure must still stop the batch")
	assert.Equal(t, int64(0), repo.checkpoint, "checkpoint must not advance past a dismiss lost to an unexpected failure")
}

// ── Supersede projections ──

func TestProjector_FoldsSummarySuperseded(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{
		"article_id":               articleID.String(),
		"new_summary_version_id":   uuid.New().String(),
		"old_summary_version_id":   uuid.New().String(),
		"previous_summary_excerpt": "the old excerpt",
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "SummarySuperseded", articleID.String(), occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	itemKey := fmt.Sprintf("article:%s", articleID)
	item, ok := repo.homeItems[itemKey]
	require.True(t, ok)
	assert.Equal(t, "summary_updated", item.SupersedeState)
	require.NotNil(t, item.SupersededAt)
	assert.True(t, occurredAt.Equal(*item.SupersededAt))
	assert.Equal(t, []string{}, item.Tags, "tags must be an explicit empty slice, not nil, so the merge-safe upsert preserves the existing row's tags")

	var prevRef map[string]string
	require.NoError(t, json.Unmarshal([]byte(item.PreviousRefJSON), &prevRef))
	assert.Equal(t, "the old excerpt", prevRef["previous_summary_excerpt"])
}

func TestProjector_FoldsTagSetSuperseded(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 15, 30, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{
		"article_id":             articleID.String(),
		"new_tag_set_version_id": uuid.New().String(),
		"old_tag_set_version_id": uuid.New().String(),
		"previous_tags":          []string{"golang"},
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "TagSetSuperseded", articleID.String(), occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	itemKey := fmt.Sprintf("article:%s", articleID)
	item, ok := repo.homeItems[itemKey]
	require.True(t, ok)
	assert.Equal(t, "tags_updated", item.SupersedeState)
	assert.Equal(t, []string{}, item.Tags, "tags must be an explicit empty slice, not nil — nil would serialize to null and wipe existing tags")

	var prevRef map[string][]string
	require.NoError(t, json.Unmarshal([]byte(item.PreviousRefJSON), &prevRef))
	assert.Equal(t, []string{"golang"}, prevRef["previous_tags"])
}

func TestProjector_FoldsReasonMerged(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID := uuid.New()
	itemKey := fmt.Sprintf("article:%s", articleID)
	occurredAt := time.Date(2026, 7, 14, 16, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{
		"article_id":         articleID.String(),
		"item_key":           itemKey,
		"added_codes":        []string{"pulse_need_to_know"},
		"previous_why_codes": []string{"new_unread"},
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "ReasonMerged", articleID.String(), occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	item, ok := repo.homeItems[itemKey]
	require.True(t, ok)
	assert.Equal(t, "reason_updated", item.SupersedeState)

	var prevRef map[string][]string
	require.NoError(t, json.Unmarshal([]byte(item.PreviousRefJSON), &prevRef))
	assert.Equal(t, []string{"new_unread"}, prevRef["previous_why_codes"])

	require.Len(t, item.WhyReasons, 1,
		"payload.added_codes must populate why_reasons — CountNeedToKnowItems filters why_json for "+
			"pulse_need_to_know, a code only ReasonMerged can deliver, so dropping it here makes the count permanently 0")
	assert.Equal(t, "pulse_need_to_know", item.WhyReasons[0].Code)
}

func TestProjector_ReasonMerged_FallsBackToArticleItemKeyWhenPayloadItemKeyEmpty(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 16, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{
		"article_id":         articleID.String(),
		"item_key":           "",
		"added_codes":        []string{"pulse_need_to_know"},
		"previous_why_codes": []string{},
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "ReasonMerged", articleID.String(), occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	itemKey := fmt.Sprintf("article:%s", articleID)
	item, ok := repo.homeItems[itemKey]
	assert.True(t, ok, "an empty payload.item_key must fall back to article:<article_id>")
	require.Len(t, item.WhyReasons, 1, "the item_key fallback must not come at the cost of dropping added_codes")
	assert.Equal(t, "pulse_need_to_know", item.WhyReasons[0].Code)
}
