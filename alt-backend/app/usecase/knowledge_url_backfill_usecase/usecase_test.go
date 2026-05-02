package knowledge_url_backfill_usecase

import (
	"alt/domain"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock ports ---

type mockArticlesPort struct {
	pages [][]domain.KnowledgeBackfillArticle
	calls int
	err   error
}

func (m *mockArticlesPort) ListBackfillArticles(_ context.Context, _ *time.Time, _ *uuid.UUID, _ int) ([]domain.KnowledgeBackfillArticle, error) {
	defer func() { m.calls++ }()
	if m.err != nil {
		return nil, m.err
	}
	if m.calls >= len(m.pages) {
		return nil, nil
	}
	return m.pages[m.calls], nil
}

// mockEventPort simulates the sovereign-side dedupe registry: each unique
// dedupe_key gets a fresh event_seq on first call, then returns seq=0
// (the dedupe-hit signal) on every subsequent call for that key. This
// lets us assert the usecase's SkippedDuplicate counter is honest after
// the AppendKnowledgeEventPort signature change (ADR-869 contract drift).
type mockEventPort struct {
	appended   []domain.KnowledgeEvent
	seenDedupe map[string]bool
	nextSeq    int64
	err        error
}

func (m *mockEventPort) AppendKnowledgeEvent(_ context.Context, ev domain.KnowledgeEvent) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	if m.seenDedupe == nil {
		m.seenDedupe = make(map[string]bool)
	}
	if m.seenDedupe[ev.DedupeKey] {
		return 0, nil
	}
	m.seenDedupe[ev.DedupeKey] = true
	m.appended = append(m.appended, ev)
	m.nextSeq++
	return m.nextSeq, nil
}

// --- helpers ---

func mkArticle(t *testing.T, url string) domain.KnowledgeBackfillArticle {
	t.Helper()
	return domain.KnowledgeBackfillArticle{
		ArticleID:   uuid.New(),
		UserID:      uuid.New(),
		CreatedAt:   time.Now(),
		PublishedAt: time.Now(),
		Title:       "t",
		URL:         url,
	}
}

// --- tests ---

func TestEmit_AppendsArticleUrlBackfilledForEveryHTTPArticle(t *testing.T) {
	t.Parallel()
	a1 := mkArticle(t, "https://example.com/a1")
	a2 := mkArticle(t, "http://example.com/a2")

	articles := &mockArticlesPort{pages: [][]domain.KnowledgeBackfillArticle{{a1, a2}}}
	events := &mockEventPort{}
	uc := NewUsecase(articles, events)

	res, err := uc.Emit(context.Background(), 0, false)
	require.NoError(t, err)
	assert.Equal(t, 2, res.ArticlesScanned)
	assert.Equal(t, 2, res.EventsAppended)
	assert.Equal(t, 0, res.SkippedBlockedScheme)
	require.Len(t, events.appended, 2)

	for i, ev := range events.appended {
		assert.Equal(t, domain.EventArticleUrlBackfilled, ev.EventType)
		assert.Equal(t, "knowledge-url-backfill", ev.ActorID)
		// The wire key MUST be canonical "url" — the bug class this
		// usecase exists to repair was wire-form drift on this exact
		// payload, so we assert the bytes directly rather than
		// round-tripping through the same struct.
		assert.Contains(t, string(ev.Payload), `"url":`)
		assert.NotContains(t, string(ev.Payload), `"link":`)
		// Dedupe key uses the corrective-event namespace, NOT the
		// ArticleCreated namespace — otherwise the existing dedupe
		// registry entries would silent-drop this emit. Resolved via
		// the const so future namespace bumps (v1 → v2 → …) keep this
		// assertion in sync with the producer code path.
		expectedDedupe := fmt.Sprintf(domain.DedupeKeyArticleUrlBackfill,
			[]domain.KnowledgeBackfillArticle{a1, a2}[i].ArticleID.String())
		assert.Equal(t, expectedDedupe, ev.DedupeKey)
	}
}

func TestEmit_RejectsNonHTTPSchemesAtAllowlistBoundary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		url  string
	}{
		{"javascript", "javascript:alert(1)"},
		{"data", "data:text/html,<script>"},
		{"file", "file:///etc/passwd"},
		{"vbscript", "vbscript:msgbox"},
		{"empty", ""},
		{"whitespace", "   "},
		{"protocol-relative", "//evil.com/x"},
		{"relative", "/articles/123"},
		{"malformed", "htps:::"},
		{"leading-whitespace-js", "   javascript:alert(1)"},
		{"casing-js", "JaVaScRiPt:alert(1)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := mkArticle(t, tc.url)
			articles := &mockArticlesPort{pages: [][]domain.KnowledgeBackfillArticle{{a}}}
			events := &mockEventPort{}
			uc := NewUsecase(articles, events)

			res, err := uc.Emit(context.Background(), 0, false)
			require.NoError(t, err)
			assert.Equal(t, 1, res.ArticlesScanned)
			assert.Equal(t, 0, res.EventsAppended)
			assert.Equal(t, 1, res.SkippedBlockedScheme)
			assert.Empty(t, events.appended, "blocked scheme %q must not reach the event port", tc.url)
		})
	}
}

func TestEmit_DryRunCountsButDoesNotAppend(t *testing.T) {
	t.Parallel()
	a1 := mkArticle(t, "https://example.com/a")
	a2 := mkArticle(t, "javascript:alert(1)")

	articles := &mockArticlesPort{pages: [][]domain.KnowledgeBackfillArticle{{a1, a2}}}
	events := &mockEventPort{}
	uc := NewUsecase(articles, events)

	res, err := uc.Emit(context.Background(), 0, true /* dryRun */)
	require.NoError(t, err)
	assert.Equal(t, 2, res.ArticlesScanned)
	assert.Equal(t, 0, res.EventsAppended, "dry_run must not append")
	assert.Equal(t, 1, res.SkippedBlockedScheme, "scheme allowlist still applied in dry_run")
	assert.Empty(t, events.appended)
}

func TestEmit_RespectsMaxArticlesAndReportsMoreRemaining(t *testing.T) {
	t.Parallel()
	page := []domain.KnowledgeBackfillArticle{
		mkArticle(t, "https://example.com/a"),
		mkArticle(t, "https://example.com/b"),
		mkArticle(t, "https://example.com/c"),
	}
	articles := &mockArticlesPort{pages: [][]domain.KnowledgeBackfillArticle{page}}
	events := &mockEventPort{}
	uc := NewUsecase(articles, events)

	res, err := uc.Emit(context.Background(), 2, false)
	require.NoError(t, err)
	assert.Equal(t, 2, res.ArticlesScanned)
	assert.Equal(t, 2, res.EventsAppended)
	assert.True(t, res.MoreRemaining, "max_articles cap should surface MoreRemaining for the operator")
	assert.Len(t, events.appended, 2)
}

func TestEmit_StopsCleanlyOnEventPortError(t *testing.T) {
	t.Parallel()
	a := mkArticle(t, "https://example.com/a")
	articles := &mockArticlesPort{pages: [][]domain.KnowledgeBackfillArticle{{a}}}
	events := &mockEventPort{err: errors.New("sovereign down")}
	uc := NewUsecase(articles, events)

	res, err := uc.Emit(context.Background(), 0, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "sovereign down")
	assert.Equal(t, 1, res.ArticlesScanned)
	assert.Equal(t, 0, res.EventsAppended)
}

// TestEmit_ReportsSkippedDuplicateAccurately is the regression guard for
// the ADR-869 contract drift: when the sovereign dedupe registry already
// has a row for `article-url-backfill:<id>`, the AppendKnowledgeEvent RPC
// returns event_seq=0 (no new event recorded). The usecase MUST count
// that as SkippedDuplicate, not as EventsAppended. Before the port
// signature change the usecase had no way to distinguish appended vs
// dedupe-hit and reported all calls as "appended".
func TestEmit_ReportsSkippedDuplicateAccurately(t *testing.T) {
	t.Parallel()
	a1 := mkArticle(t, "https://example.com/a1")
	a2 := mkArticle(t, "https://example.com/a2")
	a3 := mkArticle(t, "https://example.com/a3")

	// Two pages so the second Emit() actually re-walks (exhausted page
	// cursors return nil — the same articles must be fed back in).
	articles := &mockArticlesPort{pages: [][]domain.KnowledgeBackfillArticle{{a1, a2, a3}, {a1, a2, a3}}}
	events := &mockEventPort{}
	uc := NewUsecase(articles, events)

	res1, err := uc.Emit(context.Background(), 0, false)
	require.NoError(t, err)
	assert.Equal(t, 3, res1.EventsAppended, "first run appends every article")
	assert.Equal(t, 0, res1.SkippedDuplicate)

	res2, err := uc.Emit(context.Background(), 0, false)
	require.NoError(t, err)
	assert.Equal(t, 0, res2.EventsAppended, "second run sees every dedupe key already present")
	assert.Equal(t, 3, res2.SkippedDuplicate, "every replay must be reported as duplicate")
}

// TestEmit_PassesOriginalOccurredAtFromArticleCreatedAt pins the Verraes
// multi-temporal events shape: the corrective event's payload carries
// the article's *original* observed time (= articles.created_at) so
// future projectors can distinguish recorded-time (= when the operator
// re-emitted) from fact-time (= when the article was first seen).
func TestEmit_PassesOriginalOccurredAtFromArticleCreatedAt(t *testing.T) {
	t.Parallel()
	originalCreatedAt := time.Date(2026, 1, 15, 8, 30, 0, 0, time.UTC)
	a := domain.KnowledgeBackfillArticle{
		ArticleID:   uuid.New(),
		UserID:      uuid.New(),
		CreatedAt:   originalCreatedAt,
		PublishedAt: originalCreatedAt,
		Title:       "old article",
		URL:         "https://example.com/old",
	}
	articles := &mockArticlesPort{pages: [][]domain.KnowledgeBackfillArticle{{a}}}
	events := &mockEventPort{}
	uc := NewUsecase(articles, events)

	_, err := uc.Emit(context.Background(), 0, false)
	require.NoError(t, err)
	require.Len(t, events.appended, 1)

	// Assert directly on the marshalled bytes — the bug class this
	// payload exists to prevent (PM-2026-041) was wire-form drift.
	body := string(events.appended[0].Payload)
	assert.Contains(t, body, `"original_occurred_at":"2026-01-15T08:30:00Z"`,
		"corrective payload must carry original observed time as RFC3339")
}

// TestEmit_OnContextCanceledDuringAppend_ReturnsPartialNotError verifies that
// the BFF 30s deadline (or any other parent ctx cancel) is treated as partial
// progress, not a hard failure. Already-appended events are durable on the
// sovereign side so the operator can re-run safely; the user no longer sees
// "Failed to emit article URL backfill." for what is actually a successful
// partial run.
func TestEmit_OnContextCanceledDuringAppend_ReturnsPartialNotError(t *testing.T) {
	t.Parallel()
	a1 := mkArticle(t, "https://example.com/a1")
	a2 := mkArticle(t, "https://example.com/a2")
	articles := &mockArticlesPort{pages: [][]domain.KnowledgeBackfillArticle{{a1, a2}}}

	// Event port returns context.Canceled on the second append (mid-iteration
	// cancel) — mimicking BFF deadline tripping during the upstream RPC.
	calls := 0
	events := &cancelOnNthCall{n: 2}

	uc := NewUsecase(articles, events)
	res, err := uc.Emit(context.Background(), 0, false)

	require.NoError(t, err, "ctx cancel must surface as partial, not error")
	assert.True(t, res.MoreRemaining, "MoreRemaining should signal the operator to re-run")
	assert.Equal(t, 1, res.EventsAppended, "first append should have stuck")
	assert.Equal(t, 1, res.ArticlesScanned, "the canceled iteration should not count as scanned")
	_ = calls // silence unused
}

// TestEmit_OnContextCanceledStringFromConnect_AlsoTreatedAsPartial covers
// the case where the cancel comes back via connect-go wrapped as a
// connect.CodeCanceled error string ("canceled: context canceled") rather
// than the std-lib context.Canceled sentinel — exactly what we observed in
// alt-backend logs for the 02:42 BFF-deadline incident.
func TestEmit_OnContextCanceledStringFromConnect_AlsoTreatedAsPartial(t *testing.T) {
	t.Parallel()
	a1 := mkArticle(t, "https://example.com/a1")
	articles := &mockArticlesPort{pages: [][]domain.KnowledgeBackfillArticle{{a1}}}
	events := &mockEventPort{err: errors.New("sovereign AppendKnowledgeEvent: canceled: context canceled")}

	uc := NewUsecase(articles, events)
	res, err := uc.Emit(context.Background(), 0, false)

	require.NoError(t, err)
	assert.True(t, res.MoreRemaining)
	assert.Equal(t, 0, res.EventsAppended)
}

// cancelOnNthCall returns context.Canceled on the n-th AppendKnowledgeEvent
// call, simulating BFF deadline tripping mid-iteration.
type cancelOnNthCall struct {
	mockEventPort
	n     int
	count int
}

func (c *cancelOnNthCall) AppendKnowledgeEvent(ctx context.Context, ev domain.KnowledgeEvent) (int64, error) {
	c.count++
	if c.count == c.n {
		return 0, context.Canceled
	}
	return c.mockEventPort.AppendKnowledgeEvent(ctx, ev)
}
