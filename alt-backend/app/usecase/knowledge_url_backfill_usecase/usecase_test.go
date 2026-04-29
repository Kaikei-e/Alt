package knowledge_url_backfill_usecase

import (
	"alt/domain"
	"context"
	"errors"
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

type mockEventPort struct {
	appended []domain.KnowledgeEvent
	err      error
}

func (m *mockEventPort) AppendKnowledgeEvent(_ context.Context, ev domain.KnowledgeEvent) error {
	if m.err != nil {
		return m.err
	}
	m.appended = append(m.appended, ev)
	return nil
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
		// Dedupe key uses the ADR-000868 namespace, NOT the
		// ArticleCreated namespace — otherwise the existing dedupe
		// registry entries would silent-drop this emit.
		expectedDedupe := "article-url-backfill:" + []domain.KnowledgeBackfillArticle{a1, a2}[i].ArticleID.String()
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
