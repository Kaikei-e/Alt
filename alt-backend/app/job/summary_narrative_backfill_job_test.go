package job

import (
	"alt/domain"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBackfillListSummaryTitlesPort struct {
	rows []domain.KnowledgeBackfillSummaryTitle
}

func (m *mockBackfillListSummaryTitlesPort) ListBackfillSummaryTitles(
	_ context.Context, _ *time.Time, _ *uuid.UUID, _ int,
) ([]domain.KnowledgeBackfillSummaryTitle, error) {
	return m.rows, nil
}

func TestProcessSummaryNarrativeBatch_AppendsPatchEventsAndAdvancesCursor(t *testing.T) {
	svID := uuid.New()
	articleID := uuid.New()
	userID := uuid.New()
	tenantID := userID // single-tenant convention
	generatedAt := time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)

	updatePort := &mockBackfillUpdatePort{}
	eventPort := &mockBackfillEventPort{}
	err := processSummaryNarrativeBatch(
		context.Background(),
		updatePort,
		&mockBackfillListJobsPort{jobs: []domain.KnowledgeBackfillJob{{
			JobID:       uuid.New(),
			Kind:        domain.BackfillKindSummaryNarratives,
			Status:      domain.BackfillStatusPending,
			TotalEvents: 1,
		}}},
		&mockBackfillListSummaryTitlesPort{rows: []domain.KnowledgeBackfillSummaryTitle{{
			SummaryVersionID: svID,
			ArticleID:        articleID,
			UserID:           userID,
			TenantID:         tenantID,
			Title:            "Discovered Title",
			GeneratedAt:      generatedAt,
		}}},
		eventPort,
	)
	require.NoError(t, err)
	require.Len(t, updatePort.updated, 2)
	assert.Equal(t, domain.BackfillStatusRunning, updatePort.updated[0].Status)
	assert.Equal(t, domain.BackfillStatusCompleted, updatePort.updated[1].Status)
	assert.Equal(t, 1, updatePort.updated[1].ProcessedEvents)

	// Cursor must advance on (generated_at, summary_version_id) pair.
	require.NotNil(t, updatePort.updated[1].CursorDate)
	require.NotNil(t, updatePort.updated[1].CursorArticleID)
	assert.Equal(t, generatedAt, *updatePort.updated[1].CursorDate)
	assert.Equal(t, svID, *updatePort.updated[1].CursorArticleID)

	// One synthetic event with the canonical dedupe_key + article_title.
	require.Len(t, eventPort.events, 1)
	ev := eventPort.events[0]
	assert.Equal(t, domain.EventSummaryNarrativeBackfilled, ev.EventType)
	assert.Equal(t, "summary-narrative-backfill:"+svID.String(), ev.DedupeKey)
	assert.Equal(t, articleID.String(), ev.AggregateID)
	assert.Equal(t, generatedAt, ev.OccurredAt,
		"OccurredAt MUST be the summary's GeneratedAt — event-time purity (#3) "+
			"forbids time.Now() in business-fact times")

	var payload map[string]string
	require.NoError(t, json.Unmarshal(ev.Payload, &payload))
	assert.Equal(t, svID.String(), payload["summary_version_id"])
	assert.Equal(t, articleID.String(), payload["article_id"])
	assert.Equal(t, "Discovered Title", payload["article_title"])
}

func TestProcessSummaryNarrativeBatch_IgnoresArticleKindRows(t *testing.T) {
	// A job row of kind='articles' must be ignored by this job — the
	// dedicated KnowledgeBackfillJob walks that stream. The kind discriminator
	// (ADR-000846) is the only thing keeping the two jobs from competing.
	updatePort := &mockBackfillUpdatePort{}
	eventPort := &mockBackfillEventPort{}
	err := processSummaryNarrativeBatch(
		context.Background(),
		updatePort,
		&mockBackfillListJobsPort{jobs: []domain.KnowledgeBackfillJob{{
			JobID:  uuid.New(),
			Kind:   domain.BackfillKindArticles,
			Status: domain.BackfillStatusRunning,
		}}},
		&mockBackfillListSummaryTitlesPort{},
		eventPort,
	)
	require.NoError(t, err)
	assert.Empty(t, updatePort.updated, "must NOT touch other-kind jobs")
	assert.Empty(t, eventPort.events, "must NOT emit events when no kind match")
}

func TestProcessSummaryNarrativeBatch_CompletesWhenNoRowsRemain(t *testing.T) {
	updatePort := &mockBackfillUpdatePort{}
	err := processSummaryNarrativeBatch(
		context.Background(),
		updatePort,
		&mockBackfillListJobsPort{jobs: []domain.KnowledgeBackfillJob{{
			JobID:       uuid.New(),
			Kind:        domain.BackfillKindSummaryNarratives,
			Status:      domain.BackfillStatusRunning,
			TotalEvents: 5,
		}}},
		&mockBackfillListSummaryTitlesPort{},
		&mockBackfillEventPort{},
	)
	require.NoError(t, err)
	require.Len(t, updatePort.updated, 1)
	assert.Equal(t, domain.BackfillStatusCompleted, updatePort.updated[0].Status)
}

func TestGenerateSummaryNarrativeBackfilledEvent_CanonicalShape(t *testing.T) {
	svID := uuid.New()
	articleID := uuid.New()
	userID := uuid.New()
	generatedAt := time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)

	ev := GenerateSummaryNarrativeBackfilledEvent(domain.KnowledgeBackfillSummaryTitle{
		SummaryVersionID: svID,
		ArticleID:        articleID,
		UserID:           userID,
		TenantID:         userID,
		Title:            "Backfilled",
		GeneratedAt:      generatedAt,
	})

	assert.Equal(t, domain.EventSummaryNarrativeBackfilled, ev.EventType)
	assert.Equal(t, domain.AggregateArticle, ev.AggregateType)
	assert.Equal(t, articleID.String(), ev.AggregateID)
	assert.Equal(t, "summary-narrative-backfill:"+svID.String(), ev.DedupeKey)
	assert.Equal(t, generatedAt, ev.OccurredAt)
	require.NotNil(t, ev.UserID)
	assert.Equal(t, userID, *ev.UserID)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(ev.Payload, &payload))
	assert.Equal(t, "Backfilled", payload["article_title"])
}
