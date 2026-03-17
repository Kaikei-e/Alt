package job

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockEventsPort struct {
	events []domain.KnowledgeEvent
	err    error
}

func (m *mockEventsPort) ListKnowledgeEventsSince(_ context.Context, _ int64, _ int) ([]domain.KnowledgeEvent, error) {
	return m.events, m.err
}

type mockCheckpointPort struct {
	lastSeq    int64
	getErr     error
	updateErr  error
	updatedSeq int64
}

func (m *mockCheckpointPort) GetProjectionCheckpoint(_ context.Context, _ string) (int64, error) {
	return m.lastSeq, m.getErr
}

func (m *mockCheckpointPort) UpdateProjectionCheckpoint(_ context.Context, _ string, lastSeq int64) error {
	m.updatedSeq = lastSeq
	return m.updateErr
}

type mockHomeItemsPort struct {
	upserted []domain.KnowledgeHomeItem
	err      error
}

func (m *mockHomeItemsPort) UpsertKnowledgeHomeItem(_ context.Context, item domain.KnowledgeHomeItem) error {
	if m.err != nil {
		return m.err
	}
	m.upserted = append(m.upserted, item)
	return nil
}

type mockDigestPort struct {
	upserted []domain.TodayDigest
	err      error
}

func (m *mockDigestPort) UpsertTodayDigest(_ context.Context, digest domain.TodayDigest) error {
	if m.err != nil {
		return m.err
	}
	m.upserted = append(m.upserted, digest)
	return nil
}

func TestKnowledgeProjectorJob_NoEvents(t *testing.T) {
	logger.InitLogger()

	eventsPort := &mockEventsPort{events: nil}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort)
	err := fn(context.Background())

	require.NoError(t, err)
	assert.Empty(t, homeItemsPort.upserted)
	assert.Equal(t, int64(0), checkpointPort.updatedSeq)
}

func TestKnowledgeProjectorJob_ArticleCreated(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	articleID := uuid.New()
	payload, _ := json.Marshal(articleCreatedPayload{
		ArticleID:   articleID.String(),
		Title:       "Test Article",
		PublishedAt: "2026-03-17T10:00:00Z",
		TenantID:    tenantID.String(),
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{
				EventID:       uuid.New(),
				EventSeq:      1,
				TenantID:      tenantID,
				EventType:     domain.EventArticleCreated,
				AggregateType: domain.AggregateArticle,
				AggregateID:   articleID.String(),
				Payload:       payload,
			},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)
	assert.Equal(t, "article:"+articleID.String(), homeItemsPort.upserted[0].ItemKey)
	assert.Equal(t, domain.ItemArticle, homeItemsPort.upserted[0].ItemType)
	assert.Equal(t, "Test Article", homeItemsPort.upserted[0].Title)
	assert.Len(t, homeItemsPort.upserted[0].WhyReasons, 1)
	assert.Equal(t, domain.WhyNewUnread, homeItemsPort.upserted[0].WhyReasons[0].Code)
	assert.Equal(t, int64(1), checkpointPort.updatedSeq)
}

func TestKnowledgeProjectorJob_CheckpointAdvances(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	payload1, _ := json.Marshal(articleCreatedPayload{
		ArticleID: uuid.New().String(),
		Title:     "Article 1",
	})
	payload2, _ := json.Marshal(articleCreatedPayload{
		ArticleID: uuid.New().String(),
		Title:     "Article 2",
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 5, TenantID: tenantID, EventType: domain.EventArticleCreated, AggregateType: domain.AggregateArticle, AggregateID: "a1", Payload: payload1},
			{EventID: uuid.New(), EventSeq: 10, TenantID: tenantID, EventType: domain.EventArticleCreated, AggregateType: domain.AggregateArticle, AggregateID: "a2", Payload: payload2},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 4}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort)
	err := fn(context.Background())

	require.NoError(t, err)
	assert.Equal(t, int64(10), checkpointPort.updatedSeq) // Checkpoint advances to max seq
	assert.Len(t, homeItemsPort.upserted, 2)
}
