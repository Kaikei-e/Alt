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

// ── ArticleCreated ──

func TestProjector_FoldsArticleCreated(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	publishedAt := occurredAt.Add(-2 * time.Hour)

	payload := mustJSON(t, map[string]any{
		"article_id":   articleID.String(),
		"title":        "Rust async runtimes compared",
		"published_at": publishedAt.Format(time.RFC3339),
		"url":          "https://example.com/rust-async",
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "ArticleCreated", articleID.String(), occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	itemKey := fmt.Sprintf("article:%s", articleID)
	item, ok := repo.homeItems[itemKey]
	require.True(t, ok, "ArticleCreated must upsert a knowledge_home_items row")
	assert.Equal(t, "article", item.ItemType)
	assert.Equal(t, "Rust async runtimes compared", item.Title)
	assert.Equal(t, "https://example.com/rust-async", item.URL)
	assert.Equal(t, "pending", item.SummaryState)
	require.NotNil(t, item.PrimaryRefID)
	assert.Equal(t, articleID, *item.PrimaryRefID)
	require.Len(t, item.WhyReasons, 1)
	assert.Equal(t, "new_unread", item.WhyReasons[0].Code)

	require.NotNil(t, item.FreshnessAt)
	assert.True(t, occurredAt.Equal(*item.FreshnessAt), "freshness_at must derive from event.OccurredAt, not wall clock")
	assert.True(t, occurredAt.Equal(item.GeneratedAt), "generated_at must derive from event.OccurredAt")
	assert.True(t, occurredAt.Equal(item.UpdatedAt), "updated_at must derive from event.OccurredAt")
	assert.Equal(t, 0.5, item.Score, "score must be a fixed quality baseline, not a freshness decay computed once at ingest")
	assert.Equal(t, "max", item.ScoreOp, "a baseline quality score must only ever raise the stored floor, never overwrite a higher one")

	digest, ok := repo.digests[user.String()]
	require.True(t, ok, "ArticleCreated must upsert today_digest_view (new_articles/unsummarized_articles)")
	assert.Equal(t, 1, digest.NewArticles)
	assert.Equal(t, 1, digest.UnsummarizedArticles)
}

// TestProjector_ArticleCreated_ScoreIsIndependentOfIngestTimeStaleness pins
// the fix for the frozen-ranking defect: the old formula computed a decay of
// (event.OccurredAt - published_at) — i.e. how stale the article already was
// AT INGEST — and baked that one-time snapshot into the stored score forever
// (score merge only ever ratchets up, see repository.go). Two articles
// ingested at the same instant must get the identical score regardless of
// how old their published_at already was, because staleness-since-publish is
// now a read-time concern (GetKnowledgeHomeItems), not a stored fact.
func TestProjector_ArticleCreated_ScoreIsIndependentOfIngestTimeStaleness(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	occurredAt := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	freshArticle := uuid.New()
	staleArticle := uuid.New()

	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "ArticleCreated", freshArticle.String(), occurredAt, tenant, user, mustJSON(t, map[string]any{
			"article_id":   freshArticle.String(),
			"title":        "Published seconds ago",
			"published_at": occurredAt.Add(-10 * time.Second).Format(time.RFC3339),
			"url":          "https://example.com/fresh",
		})),
		homeEvent(2, "ArticleCreated", staleArticle.String(), occurredAt, tenant, user, mustJSON(t, map[string]any{
			"article_id":   staleArticle.String(),
			"title":        "Published 90 days ago",
			"published_at": occurredAt.Add(-90 * 24 * time.Hour).Format(time.RFC3339),
			"url":          "https://example.com/stale",
		})),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	fresh, ok := repo.homeItems[fmt.Sprintf("article:%s", freshArticle)]
	require.True(t, ok)
	stale, ok := repo.homeItems[fmt.Sprintf("article:%s", staleArticle)]
	require.True(t, ok)
	assert.Equal(t, fresh.Score, stale.Score,
		"score must not depend on published_at's age at ingest time — that is a read-time ranking concern")
}

// TestProjector_ArticleCreated_TodayDigestFailureStallsTheCheckpoint used to
// pin the opposite: the fold logged the digest failure at WARN and returned
// nil, so the checkpoint advanced past the event. That is fine for a side
// effect that can be recomputed, and today_digest_view cannot be — the write
// is an *additive delta*, so an event folded past is a counter increment that
// no later event and no reprojection will ever contribute again (a rebuild
// truncates and replays the same log, hitting the same failure). Swallowing
// it also made a producer/consumer schema skew — exactly the one that shipped
// when the driver started requiring last_event_seq — indistinguishable from a
// healthy run, which is what Alt Rule 8 forbids. A digest failure therefore
// stops the batch and leaves the event in the log to be retried.
func TestProjector_ArticleCreated_TodayDigestFailureStallsTheCheckpoint(t *testing.T) {
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
	repo.todayDigestErr = fmt.Errorf("today_digest_view unavailable")
	p := NewProjector(repo, nil, Config{})

	err := p.RunBatch(context.Background())
	require.Error(t, err, "a lost today_digest delta must fail the batch, not be logged and forgotten")
	assert.Contains(t, err.Error(), "today_digest_view unavailable")

	itemKey := fmt.Sprintf("article:%s", articleID)
	_, ok := repo.homeItems[itemKey]
	assert.True(t, ok, "the home item upsert already succeeded and stays — the fold is idempotent on retry")
	assert.Equal(t, int64(0), repo.checkpoint,
		"the checkpoint must not move past an event whose counter delta was never applied")
}

// ── ArticleUrlBackfilled ──

func TestProjector_FoldsArticleUrlBackfilled(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{
		"article_id": articleID.String(),
		"url":        "https://example.com/corrected",
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "ArticleUrlBackfilled", articleID.String(), occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	itemKey := fmt.Sprintf("article:%s", articleID)
	patch, ok := repo.urlPatches[itemKey]
	require.True(t, ok, "ArticleUrlBackfilled must patch the url column")
	assert.Equal(t, "https://example.com/corrected", patch.URL)
	assert.Empty(t, repo.homeItems, "ArticleUrlBackfilled is a single-column patch — it must not go through the full UpsertKnowledgeHomeItem path")
}

func TestProjector_ArticleUrlBackfilled_RejectsNonHTTPURL(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{
		"article_id": articleID.String(),
		"url":        "javascript:alert(1)",
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "ArticleUrlBackfilled", articleID.String(), occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()), "a rejected corrective URL must be skipped, not fail the batch")

	assert.Empty(t, repo.urlPatches, "a non-http(s) URL must never reach PatchKnowledgeHomeItemURL")
	assert.Equal(t, int64(1), repo.checkpoint, "checkpoint still advances past a skipped corrective event")
}

// ── SummaryVersionCreated (design change: no alt-db read) ──

func TestProjector_FoldsSummaryVersionCreated_UsesPayloadSummaryTextDirectly(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 11, 0, 0, 0, time.UTC)

	// Longer than the legacy 200-char excerpt truncation to pin the "そのまま"
	// (used as-is) contract: the payload's summary_text becomes the excerpt
	// verbatim, with no re-fetch from alt-db and no truncation.
	longText := ""
	for i := 0; i < 30; i++ {
		longText += "0123456789"
	}
	require.Greater(t, len(longText), 200)

	payload := mustJSON(t, map[string]any{
		"summary_version_id": uuid.New().String(),
		"article_id":         articleID.String(),
		"summary_text":       longText,
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "SummaryVersionCreated", articleID.String(), occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	itemKey := fmt.Sprintf("article:%s", articleID)
	item, ok := repo.homeItems[itemKey]
	require.True(t, ok)
	assert.Equal(t, longText, item.SummaryExcerpt, "summary_text from the payload must be used as-is (no alt-db GetSummaryVersionByID round trip, no truncation)")
	assert.Equal(t, "ready", item.SummaryState)
	assert.Equal(t, "max", item.ScoreOp, "a summary-ready boost must only ever raise the stored floor, never overwrite a higher one")
	var codes []string
	for _, r := range item.WhyReasons {
		codes = append(codes, r.Code)
	}
	assert.Contains(t, codes, "summary_completed")

	digest, ok := repo.digests[user.String()]
	require.True(t, ok, "SummaryVersionCreated must upsert today_digest_view")
	assert.Equal(t, 1, digest.SummarizedArticles)
	assert.Equal(t, -1, digest.UnsummarizedArticles)
}

func TestProjector_FoldsSummaryVersionCreated_EmptyTextStaysPending(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 11, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{
		"summary_version_id": uuid.New().String(),
		"article_id":         articleID.String(),
		"summary_text":       "",
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "SummaryVersionCreated", articleID.String(), occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	itemKey := fmt.Sprintf("article:%s", articleID)
	item, ok := repo.homeItems[itemKey]
	require.True(t, ok)
	assert.Empty(t, item.SummaryExcerpt)
	assert.Equal(t, "pending", item.SummaryState, "an empty summary_text must not flip summary_state to ready")
}

// TestProjector_DelayedArticleCreated_FillsBlankTitleURLAfterSummary pins the
// merge-safe repair path for the Trail article:<uuid> symptom: when
// SummaryVersionCreated arrives first (creating a Home row with blank
// title/url) and ArticleCreated is appended later (outbox emit recovery /
// orphan repair), title and url must fill in while the summary excerpt is
// preserved. Without merge-safe COALESCE the delayed ArticleCreated would
// either wipe the summary or leave the row unnameable.
func TestProjector_DelayedArticleCreated_FillsBlankTitleURLAfterSummary(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID := uuid.New()
	summaryAt := time.Date(2026, 8, 9, 10, 13, 0, 0, time.UTC)
	createdAt := summaryAt.Add(2 * time.Hour)

	summaryPayload := mustJSON(t, map[string]any{
		"summary_version_id": uuid.New().String(),
		"article_id":         articleID.String(),
		"summary_text":       "A durable summary that must survive the delayed ArticleCreated fold.",
	})
	articlePayload := mustJSON(t, map[string]any{
		"article_id":   articleID.String(),
		"title":        "Human-readable Trail title",
		"url":          "https://example.com/articles/delayed-created",
		"published_at": createdAt.Format(time.RFC3339),
		"tenant_id":    tenant.String(),
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "SummaryVersionCreated", articleID.String(), summaryAt, tenant, user, summaryPayload),
		homeEvent(2, "ArticleCreated", articleID.String(), createdAt, tenant, user, articlePayload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	itemKey := fmt.Sprintf("article:%s", articleID)
	item, ok := repo.homeItems[itemKey]
	require.True(t, ok)
	assert.Equal(t, "Human-readable Trail title", item.Title,
		"delayed ArticleCreated must fill the blank title left by SummaryVersionCreated")
	assert.Equal(t, "https://example.com/articles/delayed-created", item.URL,
		"delayed ArticleCreated must fill the blank url left by SummaryVersionCreated")
	assert.Equal(t, "A durable summary that must survive the delayed ArticleCreated fold.", item.SummaryExcerpt,
		"merge-safe upsert must preserve the summary excerpt already folded")
	assert.Equal(t, "ready", item.SummaryState)
}

// ── TagSetVersionCreated (design change: no alt-db read) ──

func TestProjector_FoldsTagSetVersionCreated_UsesPayloadTagsDirectly(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{
		"tag_set_version_id": uuid.New().String(),
		"article_id":         articleID.String(),
		"tags":               []string{"rust", "async"},
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "TagSetVersionCreated", articleID.String(), occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	itemKey := fmt.Sprintf("article:%s", articleID)
	item, ok := repo.homeItems[itemKey]
	require.True(t, ok)
	assert.Equal(t, []string{"rust", "async"}, item.Tags, "tags from the payload must be used as-is (no alt-db GetTagSetVersionByID round trip, no parseTagNames)")
	assert.Equal(t, "max", item.ScoreOp, "a tagged boost must only ever raise the stored floor, never overwrite a higher one")

	digest, ok := repo.digests[user.String()]
	require.True(t, ok, "non-empty tags must surface into today_digest_view.top_tags")
	assert.Equal(t, []string{"rust", "async"}, digest.TopTags)
}

func TestProjector_FoldsTagSetVersionCreated_EmptyTagsSkipsDigest(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{
		"tag_set_version_id": uuid.New().String(),
		"article_id":         articleID.String(),
		"tags":               []string{},
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "TagSetVersionCreated", articleID.String(), occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	itemKey := fmt.Sprintf("article:%s", articleID)
	_, ok := repo.homeItems[itemKey]
	require.True(t, ok, "the home item upsert still happens even with no tags")
	assert.Empty(t, repo.digests, "an empty tag set must not touch today_digest_view.top_tags")
}
