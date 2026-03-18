package job

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockReprojectRunsPort struct {
	runs    []domain.ReprojectRun
	updated []*domain.ReprojectRun
	err     error
}

func (m *mockReprojectRunsPort) ListReprojectRuns(_ context.Context, _ string, _ int) ([]domain.ReprojectRun, error) {
	return m.runs, m.err
}

func (m *mockReprojectRunsPort) GetReprojectRun(_ context.Context, _ uuid.UUID) (*domain.ReprojectRun, error) {
	if len(m.runs) == 0 {
		return nil, m.err
	}
	run := m.runs[0]
	return &run, m.err
}

func (m *mockReprojectRunsPort) UpdateReprojectRun(_ context.Context, run *domain.ReprojectRun) error {
	if m.err != nil {
		return m.err
	}
	clone := *run
	clone.CheckpointPayload = append([]byte(nil), run.CheckpointPayload...)
	clone.StatsJSON = append([]byte(nil), run.StatsJSON...)
	clone.DiffSummaryJSON = append([]byte(nil), run.DiffSummaryJSON...)
	m.updated = append(m.updated, &clone)
	return nil
}

func TestKnowledgeReprojectJob_ReplaysDismissIntoTargetVersion(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	tenantID := uuid.New()
	articleID := uuid.New()
	itemKey := "article:" + articleID.String()
	dismissedAt := time.Date(2026, 3, 18, 14, 21, 33, 0, time.UTC)

	runID := uuid.New()
	runsPort := &mockReprojectRunsPort{
		runs: []domain.ReprojectRun{
			{
				ReprojectRunID:    runID,
				ProjectionName:    "knowledge_home",
				FromVersion:       "v1",
				ToVersion:         "v2",
				Mode:              domain.ReprojectModeFull,
				Status:            domain.ReprojectStatusRunning,
				CheckpointPayload: json.RawMessage(`{}`),
				StatsJSON:         json.RawMessage(`{}`),
			},
		},
	}

	dismissPayload, _ := json.Marshal(map[string]string{"item_key": itemKey})
	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{
				EventID:       uuid.New(),
				EventSeq:      1,
				TenantID:      tenantID,
				UserID:        &userID,
				EventType:     domain.EventArticleCreated,
				AggregateType: domain.AggregateArticle,
				AggregateID:   articleID.String(),
				Payload:       mustJSON(articleCreatedPayload{ArticleID: articleID.String(), Title: "Replay target", PublishedAt: "2026-03-18T10:00:00Z"}),
			},
			{
				EventID:       uuid.New(),
				EventSeq:      2,
				TenantID:      tenantID,
				UserID:        &userID,
				EventType:     domain.EventHomeItemDismissed,
				AggregateType: domain.AggregateHomeSession,
				AggregateID:   itemKey,
				Payload:       dismissPayload,
				OccurredAt:    dismissedAt,
			},
		},
	}
	homeItemsPort := &mockHomeItemsPort{}

	fn := KnowledgeReprojectJob(runsPort, runsPort, runsPort, eventsPort, &mockCheckpointPort{}, &mockCheckpointPort{}, homeItemsPort, &mockDigestPort{}, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)
	assert.Equal(t, 2, homeItemsPort.upserted[0].ProjectionVersion)
	require.Len(t, homeItemsPort.dismissed, 1)
	assert.Equal(t, itemKey, homeItemsPort.dismissed[0].itemKey)
	assert.NotNil(t, homeItemsPort.items[itemKey].DismissedAt)
	require.NotEmpty(t, runsPort.updated)
	assert.JSONEq(t, `{"last_event_seq":2}`, string(runsPort.updated[len(runsPort.updated)-1].CheckpointPayload))
}

func TestKnowledgeReprojectJob_CompletesWhenNoEventsRemain(t *testing.T) {
	logger.InitLogger()

	runID := uuid.New()
	runsPort := &mockReprojectRunsPort{
		runs: []domain.ReprojectRun{
			{
				ReprojectRunID:    runID,
				ProjectionName:    "knowledge_home",
				FromVersion:       "v1",
				ToVersion:         "v2",
				Mode:              domain.ReprojectModeFull,
				Status:            domain.ReprojectStatusRunning,
				CheckpointPayload: json.RawMessage(`{"last_event_seq":12}`),
				StatsJSON:         json.RawMessage(`{"events_processed":12}`),
			},
		},
	}

	fn := KnowledgeReprojectJob(runsPort, runsPort, runsPort, &mockEventsPort{}, &mockCheckpointPort{}, &mockCheckpointPort{}, &mockHomeItemsPort{}, &mockDigestPort{}, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.NotEmpty(t, runsPort.updated)
	last := runsPort.updated[len(runsPort.updated)-1]
	assert.Equal(t, domain.ReprojectStatusSwappable, last.Status)
	assert.NotNil(t, last.FinishedAt)
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
