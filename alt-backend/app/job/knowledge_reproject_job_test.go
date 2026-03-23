package job

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"fmt"
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

func TestKnowledgeReprojectJob_ProcessesMultipleBatchesPerTick(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	tenantID := uuid.New()

	// Create 5 events — with reprojectBatchSize=2000 they'd all fit in one batch,
	// but we use a small mock set to verify the loop continues until events are exhausted.
	var allEvents []domain.KnowledgeEvent
	for i := 1; i <= 5; i++ {
		articleID := uuid.New()
		allEvents = append(allEvents, domain.KnowledgeEvent{
			EventID:       uuid.New(),
			EventSeq:      int64(i),
			TenantID:      tenantID,
			UserID:        &userID,
			EventType:     domain.EventArticleCreated,
			AggregateType: domain.AggregateArticle,
			AggregateID:   articleID.String(),
			Payload: mustJSON(articleCreatedPayload{
				ArticleID:   articleID.String(),
				Title:       fmt.Sprintf("Article %d", i),
				PublishedAt: "2026-03-18T10:00:00Z",
			}),
		})
	}

	// Mock that returns events in 2 batches: first call returns first 3, second call returns last 2, third returns empty
	callCount := 0
	eventsPort := &mockBatchEventsPort{
		batches: [][]domain.KnowledgeEvent{
			allEvents[:3],
			allEvents[3:],
			{}, // empty signals completion
		},
		callCount: &callCount,
	}

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
	homeItemsPort := &mockHomeItemsPort{}

	fn := KnowledgeReprojectJob(runsPort, runsPort, runsPort, eventsPort, &mockCheckpointPort{}, &mockCheckpointPort{}, homeItemsPort, &mockDigestPort{}, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	// The events port should have been called 3 times (2 with data + 1 empty)
	assert.Equal(t, 3, callCount, "should have fetched multiple batches in a single tick")
	// All 5 events should have been projected
	assert.Len(t, homeItemsPort.upserted, 5)
	// Run should be swappable since we exhausted all events
	require.NotEmpty(t, runsPort.updated)
	last := runsPort.updated[len(runsPort.updated)-1]
	assert.Equal(t, domain.ReprojectStatusSwappable, last.Status)
	// Stats should show all 5 processed
	var stats domain.ReprojectStats
	_ = json.Unmarshal(last.StatsJSON, &stats)
	assert.Equal(t, int64(5), stats.EventsProcessed)
}

func TestKnowledgeReprojectJob_RespectsContextDeadline(t *testing.T) {
	logger.InitLogger()

	callCount := 0
	eventsPort := &mockBatchEventsPort{
		batches: [][]domain.KnowledgeEvent{
			{{EventSeq: 1}}, // should never be reached
		},
		callCount: &callCount,
	}

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

	// Deadline already within safety margin — loop should not fetch any events
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(reprojectLoopSafetyMargin-1*time.Second))
	defer cancel()

	fn := KnowledgeReprojectJob(runsPort, runsPort, runsPort, eventsPort, &mockCheckpointPort{}, &mockCheckpointPort{}, &mockHomeItemsPort{}, &mockDigestPort{}, nil, nil)
	err := fn(ctx)

	require.NoError(t, err)
	// No events should have been fetched since deadline was too close
	assert.Equal(t, 0, callCount, "should not fetch events when deadline is near")
	// Run status update should have been called (pending->running transition already happened above,
	// but the batch loop exited early without marking swappable)
	// The run should remain running — no update to swappable
	for _, u := range runsPort.updated {
		assert.NotEqual(t, domain.ReprojectStatusSwappable, u.Status, "should not transition to swappable")
	}
}

// mockBatchEventsPort returns different event slices on successive calls.
type mockBatchEventsPort struct {
	batches   [][]domain.KnowledgeEvent
	callCount *int
}

func (m *mockBatchEventsPort) ListKnowledgeEventsSince(_ context.Context, _ int64, _ int) ([]domain.KnowledgeEvent, error) {
	idx := *m.callCount
	*m.callCount++
	if idx < len(m.batches) {
		return m.batches[idx], nil
	}
	return nil, nil
}

func TestKnowledgeReprojectJob_RoutesThroughSovereign(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	tenantID := uuid.New()
	articleID := uuid.New()

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
				Payload: mustJSON(articleCreatedPayload{
					ArticleID:   articleID.String(),
					Title:       "Sovereign routed",
					PublishedAt: "2026-03-23T10:00:00Z",
				}),
			},
		},
	}

	mockWS := &mockWriteService{}
	homeItemsPort := &mockHomeItemsPort{}

	fn := KnowledgeReprojectJobWithWriteService(
		runsPort, runsPort, runsPort,
		eventsPort, &mockCheckpointPort{}, &mockCheckpointPort{},
		homeItemsPort, &mockDigestPort{}, nil, nil,
		mockWS,
	)
	err := fn(context.Background())

	require.NoError(t, err)
	assert.Greater(t, len(mockWS.projectionCalls), 0, "WriteService should have been called for projection mutations")
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
