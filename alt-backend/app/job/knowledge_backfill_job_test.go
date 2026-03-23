package job

import (
	"alt/domain"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBackfillUpdatePort struct {
	updated []domain.KnowledgeBackfillJob
}

func (m *mockBackfillUpdatePort) UpdateBackfillJob(_ context.Context, job domain.KnowledgeBackfillJob) error {
	m.updated = append(m.updated, job)
	return nil
}

type mockBackfillListJobsPort struct {
	jobs []domain.KnowledgeBackfillJob
}

func (m *mockBackfillListJobsPort) ListBackfillJobs(_ context.Context) ([]domain.KnowledgeBackfillJob, error) {
	return m.jobs, nil
}

type mockBackfillListArticlesPort struct {
	articles []domain.KnowledgeBackfillArticle
}

func (m *mockBackfillListArticlesPort) ListBackfillArticles(_ context.Context, _ *time.Time, _ *uuid.UUID, _ int) ([]domain.KnowledgeBackfillArticle, error) {
	return m.articles, nil
}

type mockBackfillEventPort struct {
	events []domain.KnowledgeEvent
}

func (m *mockBackfillEventPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) error {
	m.events = append(m.events, event)
	return nil
}

func TestProcessBackfillBatch_AppendsEventsAndAdvancesCursor(t *testing.T) {
	articleID := uuid.New()
	userID := uuid.New()
	createdAt := time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)

	updatePort := &mockBackfillUpdatePort{}
	err := processBackfillBatch(
		context.Background(),
		nil,
		updatePort,
		&mockBackfillListJobsPort{jobs: []domain.KnowledgeBackfillJob{{
			JobID:       uuid.New(),
			Status:      domain.BackfillStatusPending,
			TotalEvents: 1,
		}}},
		&mockBackfillListArticlesPort{articles: []domain.KnowledgeBackfillArticle{{
			ArticleID:   articleID,
			UserID:      userID,
			CreatedAt:   createdAt,
			PublishedAt: createdAt,
			Title:       "Backfilled",
		}}},
		&mockBackfillEventPort{},
	)
	require.NoError(t, err)
	require.Len(t, updatePort.updated, 2)
	assert.Equal(t, domain.BackfillStatusRunning, updatePort.updated[0].Status)
	assert.Equal(t, domain.BackfillStatusCompleted, updatePort.updated[1].Status)
	assert.Equal(t, 1, updatePort.updated[1].ProcessedEvents)
	require.NotNil(t, updatePort.updated[1].CursorArticleID)
	assert.Equal(t, articleID, *updatePort.updated[1].CursorArticleID)
}

func TestProcessBackfillBatch_CompletesWhenNoArticlesRemain(t *testing.T) {
	updatePort := &mockBackfillUpdatePort{}
	err := processBackfillBatch(
		context.Background(),
		nil,
		updatePort,
		&mockBackfillListJobsPort{jobs: []domain.KnowledgeBackfillJob{{
			JobID:       uuid.New(),
			Status:      domain.BackfillStatusRunning,
			TotalEvents: 5,
		}}},
		&mockBackfillListArticlesPort{},
		&mockBackfillEventPort{},
	)
	require.NoError(t, err)
	require.Len(t, updatePort.updated, 1)
	assert.Equal(t, domain.BackfillStatusCompleted, updatePort.updated[0].Status)
}

func TestGenerateBackfillEvent_UsesCanonicalArticleCreatedDedupeKey(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	articleID := uuid.New()
	publishedAt := time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)

	event := GenerateBackfillEvent(tenantID, &userID, articleID, "Backfilled Article", publishedAt, "https://example.com/backfilled")

	assert.Equal(t, "article-created:"+articleID.String(), event.DedupeKey)
	assert.Equal(t, articleID.String(), event.AggregateID)
}
