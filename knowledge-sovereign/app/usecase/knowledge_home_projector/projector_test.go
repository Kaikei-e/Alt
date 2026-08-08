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

// ── wire-capture types ──
//
// These mirror the exact json.RawMessage unmarshal targets already living in
// sovereign_db.Repository's mutation methods (UpsertKnowledgeHomeItem,
// DismissKnowledgeHomeItem, ClearSupersedeState, UpsertTodayDigest,
// UpsertRecallCandidate, PatchKnowledgeHomeItemURL — see
// knowledge-sovereign/app/driver/sovereign_db/repository.go and
// patch_knowledge_home_item_url.go). The projector must produce payloads
// that unmarshal correctly against these targets, because Repository is
// satisfied directly by *sovereign_db.Repository (no intermediate gateway —
// same shape as knowledge_trail_projector). Using distinct "captured*" names
// here (rather than the natural production names like
// "articleCreatedPayload") avoids colliding with the fold implementation's
// own types once projector.go grows the real logic in GREEN.

type capturedHomeItem struct {
	UserID         uuid.UUID  `json:"user_id"`
	TenantID       uuid.UUID  `json:"tenant_id"`
	ItemKey        string     `json:"item_key"`
	ItemType       string     `json:"item_type"`
	PrimaryRefID   *uuid.UUID `json:"primary_ref_id"`
	Title          string     `json:"title"`
	SummaryExcerpt string     `json:"summary_excerpt"`
	Tags           []string   `json:"tags"`
	WhyReasons     []struct {
		Code   string `json:"code"`
		Reason string `json:"reason"`
	} `json:"why_reasons"`
	Score             float64    `json:"score"`
	ScoreOp           string     `json:"score_op"`
	FreshnessAt       *time.Time `json:"freshness_at"`
	PublishedAt       *time.Time `json:"published_at"`
	LastInteractedAt  *time.Time `json:"last_interacted_at"`
	GeneratedAt       time.Time  `json:"generated_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DismissedAt       *time.Time `json:"dismissed_at"`
	ProjectionVersion int        `json:"projection_version"`
	SummaryState      string     `json:"summary_state"`
	SupersedeState    string     `json:"supersede_state"`
	SupersededAt      *time.Time `json:"superseded_at"`
	PreviousRefJSON   string     `json:"previous_ref_json"`
	URL               string     `json:"url"`
}

type capturedDismiss struct {
	UserID            string `json:"user_id"`
	ItemKey           string `json:"item_key"`
	ProjectionVersion int    `json:"projection_version"`
	DismissedAt       string `json:"dismissed_at"`
}

type capturedClearSupersede struct {
	UserID            string `json:"user_id"`
	ItemKey           string `json:"item_key"`
	ProjectionVersion int    `json:"projection_version"`
}

type capturedDigest struct {
	UserID               uuid.UUID `json:"user_id"`
	DigestDate           string    `json:"digest_date"`
	NewArticles          int       `json:"new_articles"`
	SummarizedArticles   int       `json:"summarized_articles"`
	UnsummarizedArticles int       `json:"unsummarized_articles"`
	TopTags              []string  `json:"top_tags"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type capturedRecallCandidate struct {
	UserID  uuid.UUID `json:"user_id"`
	ItemKey string    `json:"item_key"`
	Reasons []struct {
		Type        string `json:"type"`
		Description string `json:"description"`
	} `json:"reasons"`
	RecallScore       float64    `json:"recall_score"`
	NextSuggestAt     *time.Time `json:"next_suggest_at"`
	FirstEligibleAt   *time.Time `json:"first_eligible_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	ProjectionVersion int        `json:"projection_version"`
}

type capturedURLPatch struct {
	UserID            string `json:"user_id"`
	ItemKey           string `json:"item_key"`
	ProjectionVersion int    `json:"projection_version"`
	URL               string `json:"url"`
}

type capturedSnooze struct {
	UserID     string `json:"user_id"`
	ItemKey    string `json:"item_key"`
	Until      string `json:"until"`
	OccurredAt string `json:"occurred_at"`
}

type capturedRecallDismiss struct {
	UserID     string `json:"user_id"`
	ItemKey    string `json:"item_key"`
	OccurredAt string `json:"occurred_at"`
}

// ── fakeRepo ──

// fakeRepo is an in-memory stand-in for the sovereign repository, mirroring
// knowledge_trail_projector's fakeRepo pattern. Each mutation method decodes
// the json.RawMessage into its wire-capture type and records it by key, so
// tests can assert on fold outcomes without a live database. Error injection
// fields let tests exercise the non-fatal side-effect paths (today_digest /
// recall_candidate / clear_supersede failures must not fail the batch — see
// alt-backend/app/job/knowledge_projector.go's "// Non-fatal" comments).
type fakeRepo struct {
	events     []sovereign_db.KnowledgeEvent
	checkpoint int64

	homeItems        map[string]capturedHomeItem
	dismissed        map[string]capturedDismiss
	clearedSupersede map[string]int
	digests          map[string]capturedDigest
	recallCandidates map[string]capturedRecallCandidate
	urlPatches       map[string]capturedURLPatch
	snoozed          map[string]capturedSnooze
	recallDismissed  map[string]capturedRecallDismiss

	// activeProjectionVersion mirrors knowledge_projection_versions'
	// status='active' row. Defaults to version 1 so existing tests that
	// don't care about versioning keep working unchanged; set to nil to
	// simulate no active version row (must fail the batch loudly).
	activeProjectionVersion *sovereign_db.ProjectionVersion
	activeVersionErr        error

	// dismissMissingKeys mirrors the real repository's zero-rows-updated
	// outcome: item_keys listed here have no knowledge_home_items row at the
	// projector's version, so DismissKnowledgeHomeItem reports
	// sovereign_db.ErrDismissTargetNotFound exactly as Postgres would (see
	// driver/sovereign_db/repository.go's RowsAffected() == 0 branch).
	dismissMissingKeys map[string]bool

	todayDigestErr    error
	recallCandErr     error
	clearSupersedeErr error
	snoozeRecallErr   error
	dismissRecallErr  error
	dismissHomeErr    error
}

var _ Repository = (*fakeRepo)(nil)

func newFakeRepo(events []sovereign_db.KnowledgeEvent) *fakeRepo {
	return &fakeRepo{
		events:                  events,
		homeItems:               map[string]capturedHomeItem{},
		dismissed:               map[string]capturedDismiss{},
		clearedSupersede:        map[string]int{},
		digests:                 map[string]capturedDigest{},
		recallCandidates:        map[string]capturedRecallCandidate{},
		urlPatches:              map[string]capturedURLPatch{},
		snoozed:                 map[string]capturedSnooze{},
		recallDismissed:         map[string]capturedRecallDismiss{},
		dismissMissingKeys:      map[string]bool{},
		activeProjectionVersion: &sovereign_db.ProjectionVersion{Version: 1},
	}
}

func (f *fakeRepo) GetProjectionCheckpoint(_ context.Context, _ string) (int64, error) {
	return f.checkpoint, nil
}

func (f *fakeRepo) UpdateProjectionCheckpoint(_ context.Context, _ string, lastSeq int64) error {
	f.checkpoint = lastSeq
	return nil
}

func (f *fakeRepo) ListKnowledgeEventsSince(_ context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error) {
	var out []sovereign_db.KnowledgeEvent
	for _, e := range f.events {
		if e.EventSeq > afterSeq {
			out = append(out, e)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (f *fakeRepo) GetActiveProjectionVersion(_ context.Context) (*sovereign_db.ProjectionVersion, error) {
	if f.activeVersionErr != nil {
		return nil, f.activeVersionErr
	}
	return f.activeProjectionVersion, nil
}

func (f *fakeRepo) UpsertKnowledgeHomeItem(_ context.Context, payload json.RawMessage) error {
	var w capturedHomeItem
	if err := json.Unmarshal(payload, &w); err != nil {
		return fmt.Errorf("fakeRepo.UpsertKnowledgeHomeItem: %w", err)
	}
	f.homeItems[w.ItemKey] = w
	return nil
}

func (f *fakeRepo) DismissKnowledgeHomeItem(_ context.Context, payload json.RawMessage) error {
	if f.dismissHomeErr != nil {
		return f.dismissHomeErr
	}
	var w capturedDismiss
	if err := json.Unmarshal(payload, &w); err != nil {
		return fmt.Errorf("fakeRepo.DismissKnowledgeHomeItem: %w", err)
	}
	if f.dismissMissingKeys[w.ItemKey] {
		return sovereign_db.ErrDismissTargetNotFound
	}
	f.dismissed[w.ItemKey] = w
	return nil
}

func (f *fakeRepo) ClearSupersedeState(_ context.Context, payload json.RawMessage) error {
	if f.clearSupersedeErr != nil {
		return f.clearSupersedeErr
	}
	var w capturedClearSupersede
	if err := json.Unmarshal(payload, &w); err != nil {
		return fmt.Errorf("fakeRepo.ClearSupersedeState: %w", err)
	}
	f.clearedSupersede[w.ItemKey]++
	return nil
}

func (f *fakeRepo) UpsertTodayDigest(_ context.Context, payload json.RawMessage) error {
	if f.todayDigestErr != nil {
		return f.todayDigestErr
	}
	var w capturedDigest
	if err := json.Unmarshal(payload, &w); err != nil {
		return fmt.Errorf("fakeRepo.UpsertTodayDigest: %w", err)
	}
	f.digests[w.UserID.String()] = w
	return nil
}

func (f *fakeRepo) UpsertRecallCandidate(_ context.Context, payload json.RawMessage) error {
	if f.recallCandErr != nil {
		return f.recallCandErr
	}
	var w capturedRecallCandidate
	if err := json.Unmarshal(payload, &w); err != nil {
		return fmt.Errorf("fakeRepo.UpsertRecallCandidate: %w", err)
	}
	f.recallCandidates[w.ItemKey] = w
	return nil
}

func (f *fakeRepo) PatchKnowledgeHomeItemURL(_ context.Context, payload json.RawMessage) error {
	var w capturedURLPatch
	if err := json.Unmarshal(payload, &w); err != nil {
		return fmt.Errorf("fakeRepo.PatchKnowledgeHomeItemURL: %w", err)
	}
	f.urlPatches[w.ItemKey] = w
	return nil
}

func (f *fakeRepo) SnoozeRecallCandidate(_ context.Context, payload json.RawMessage) error {
	if f.snoozeRecallErr != nil {
		return f.snoozeRecallErr
	}
	var w capturedSnooze
	if err := json.Unmarshal(payload, &w); err != nil {
		return fmt.Errorf("fakeRepo.SnoozeRecallCandidate: %w", err)
	}
	f.snoozed[w.ItemKey] = w
	return nil
}

func (f *fakeRepo) DismissRecallCandidate(_ context.Context, payload json.RawMessage) error {
	if f.dismissRecallErr != nil {
		return f.dismissRecallErr
	}
	var w capturedRecallDismiss
	if err := json.Unmarshal(payload, &w); err != nil {
		return fmt.Errorf("fakeRepo.DismissRecallCandidate: %w", err)
	}
	f.recallDismissed[w.ItemKey] = w
	return nil
}

// ── event builders ──

func userPtr() *uuid.UUID { u := uuid.New(); return &u }

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func homeEvent(seq int64, eventType, aggregateID string, occurredAt time.Time, tenantID uuid.UUID, userID *uuid.UUID, payload json.RawMessage) sovereign_db.KnowledgeEvent {
	return sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    occurredAt,
		TenantID:      tenantID,
		UserID:        userID,
		EventType:     eventType,
		AggregateType: "article",
		AggregateID:   aggregateID,
		DedupeKey:     fmt.Sprintf("%s:%d", eventType, seq),
		Payload:       payload,
	}
}

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

func TestProjector_ArticleCreated_TodayDigestFailureIsNonFatal(t *testing.T) {
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
	require.NoError(t, err, "a today_digest upsert failure must not fail the batch (non-fatal side effect)")

	itemKey := fmt.Sprintf("article:%s", articleID)
	_, ok := repo.homeItems[itemKey]
	assert.True(t, ok, "the home item upsert must still succeed even when today_digest fails")
	assert.Equal(t, int64(1), repo.checkpoint, "checkpoint still advances past a non-fatal side-effect failure")
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

// ── RecallSnoozed / RecallDismissed ──
//
// alt-backend's recall_snooze_usecase/recall_dismiss_usecase append these
// events after already writing recall_candidate_view directly (write-through).
// A full TRUNCATE + reproject replay must reach the same snoozed_until /
// dismissed_at state, so the projector must fold these two event types too —
// they must not fall into the "unknown event types are silently skipped"
// default case.

func TestProjector_FoldsRecallSnoozed(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	itemKey := "article:" + uuid.New().String()
	occurredAt := time.Date(2026, 7, 14, 19, 0, 0, 0, time.UTC)
	until := occurredAt.Add(24 * time.Hour)

	payload := mustJSON(t, map[string]any{
		"item_key":      itemKey,
		"snooze_hours":  24,
		"snoozed_until": until.Format(time.RFC3339),
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "RecallSnoozed", itemKey, occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	snooze, ok := repo.snoozed[itemKey]
	require.True(t, ok, "RecallSnoozed must reach SnoozeRecallCandidate on reproject, not be silently skipped")
	assert.Equal(t, user.String(), snooze.UserID)
	gotUntil, err := time.Parse(time.RFC3339Nano, snooze.Until)
	require.NoError(t, err)
	assert.True(t, until.Equal(gotUntil))
	gotOccurredAt, err := time.Parse(time.RFC3339Nano, snooze.OccurredAt)
	require.NoError(t, err)
	assert.True(t, occurredAt.Equal(gotOccurredAt), "occurred_at must derive from event.OccurredAt, not wall clock")
	assert.Equal(t, int64(1), repo.checkpoint)
}

func TestProjector_FoldsRecallDismissed(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	itemKey := "article:" + uuid.New().String()
	occurredAt := time.Date(2026, 7, 14, 19, 30, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{
		"item_key": itemKey,
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "RecallDismissed", itemKey, occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	dismiss, ok := repo.recallDismissed[itemKey]
	require.True(t, ok, "RecallDismissed must reach DismissRecallCandidate on reproject, not be silently skipped")
	assert.Equal(t, user.String(), dismiss.UserID)
	gotOccurredAt, err := time.Parse(time.RFC3339Nano, dismiss.OccurredAt)
	require.NoError(t, err)
	assert.True(t, occurredAt.Equal(gotOccurredAt), "occurred_at must derive from event.OccurredAt, not wall clock")
	assert.Equal(t, int64(1), repo.checkpoint)
}

func TestProjector_RecallSnoozed_RepositoryFailureStopsBatch(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	itemKey := "article:" + uuid.New().String()
	occurredAt := time.Date(2026, 7, 14, 19, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{
		"item_key":      itemKey,
		"snooze_hours":  24,
		"snoozed_until": occurredAt.Add(24 * time.Hour).Format(time.RFC3339),
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "RecallSnoozed", itemKey, occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	repo.snoozeRecallErr = fmt.Errorf("recall_candidate_view unavailable")
	p := NewProjector(repo, nil, Config{})

	err := p.RunBatch(context.Background())
	require.Error(t, err, "unlike the digest/recall-candidate side effects on other events, the snooze write IS the event's entire purpose — a failure must not advance past it")
	assert.Equal(t, int64(0), repo.checkpoint, "checkpoint must not advance past a failed RecallSnoozed fold")
}

// ── checkpoint / unknown events ──

func TestProjector_SkipsUnknownEventTypeButAdvancesCheckpoint(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	occurredAt := time.Date(2026, 7, 14, 17, 0, 0, 0, time.UTC)

	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "SomeFutureEventType", "whatever", occurredAt, tenant, user, mustJSON(t, map[string]any{})),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	assert.Empty(t, repo.homeItems)
	assert.Equal(t, int64(1), repo.checkpoint, "checkpoint must still advance past an unrecognized event type")
}

func TestProjector_MalformedPayloadStopsBatchButPreservesPriorCheckpoint(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID1 := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 18, 0, 0, 0, time.UTC)

	goodPayload := mustJSON(t, map[string]any{
		"article_id": articleID1.String(),
		"title":      "Good article",
		"url":        "https://example.com/good",
	})
	badPayload := json.RawMessage(`{"article_id": not-json`)

	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "ArticleCreated", articleID1.String(), occurredAt, tenant, user, goodPayload),
		homeEvent(2, "ArticleCreated", "broken", occurredAt.Add(time.Minute), tenant, user, badPayload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})

	err := p.RunBatch(context.Background())
	require.Error(t, err, "a malformed payload must fail the batch so the event is retried, not silently dropped")

	itemKey1 := fmt.Sprintf("article:%s", articleID1)
	_, ok := repo.homeItems[itemKey1]
	assert.True(t, ok, "events processed before the malformed one must still be applied")
	assert.Equal(t, int64(1), repo.checkpoint, "checkpoint must stop at the last successfully-folded event, not skip past the failure")
}

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
