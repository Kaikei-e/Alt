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

type mockActiveVersionPort struct {
	version *domain.KnowledgeProjectionVersion
	err     error
}

func (m *mockActiveVersionPort) GetActiveVersion(_ context.Context) (*domain.KnowledgeProjectionVersion, error) {
	return m.version, m.err
}

func TestKnowledgeProjectorJob_NoEvents(t *testing.T) {
	logger.InitLogger()

	eventsPort := &mockEventsPort{events: nil}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil)
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

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)
	assert.Equal(t, "article:"+articleID.String(), homeItemsPort.upserted[0].ItemKey)
	assert.Equal(t, domain.ItemArticle, homeItemsPort.upserted[0].ItemType)
	assert.Equal(t, "Test Article", homeItemsPort.upserted[0].Title)
	assert.Len(t, homeItemsPort.upserted[0].WhyReasons, 1)
	assert.Equal(t, domain.WhyNewUnread, homeItemsPort.upserted[0].WhyReasons[0].Code)
	assert.Equal(t, int64(1), checkpointPort.updatedSeq)
	assert.Equal(t, 1, homeItemsPort.upserted[0].ProjectionVersion) // default version
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

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	assert.Equal(t, int64(10), checkpointPort.updatedSeq) // Checkpoint advances to max seq
	assert.Len(t, homeItemsPort.upserted, 2)
}

type mockSummaryVersionPort struct {
	sv  domain.SummaryVersion
	err error
}

func (m *mockSummaryVersionPort) GetLatestSummaryVersion(_ context.Context, _ uuid.UUID) (domain.SummaryVersion, error) {
	return m.sv, m.err
}

type mockRecallCandidatePort struct {
	upserted []domain.RecallCandidate
	err      error
}

func (m *mockRecallCandidatePort) UpsertRecallCandidate(_ context.Context, c domain.RecallCandidate) error {
	if m.err != nil {
		return m.err
	}
	m.upserted = append(m.upserted, c)
	return nil
}

func TestKnowledgeProjectorJob_SummaryVersionCreated_PopulatesExcerpt(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	articleID := uuid.New()
	svID := uuid.New()

	// First create the article
	articlePayload, _ := json.Marshal(articleCreatedPayload{
		ArticleID:   articleID.String(),
		Title:       "Test Article",
		PublishedAt: "2026-03-17T10:00:00Z",
	})
	// Then create the summary version event
	summaryPayload, _ := json.Marshal(summaryVersionPayload{
		SummaryVersionID: svID.String(),
		ArticleID:        articleID.String(),
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, EventType: domain.EventArticleCreated, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: articlePayload},
			{EventID: uuid.New(), EventSeq: 2, TenantID: tenantID, EventType: domain.EventSummaryVersionCreated, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: summaryPayload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}
	summaryVersionPort := &mockSummaryVersionPort{
		sv: domain.SummaryVersion{
			SummaryVersionID: svID,
			ArticleID:        articleID,
			SummaryText:      "This is a detailed summary of the test article covering important topics.",
		},
	}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, summaryVersionPort, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 2)
	// Second upsert is the summary version event - should have excerpt
	assert.NotEmpty(t, homeItemsPort.upserted[1].SummaryExcerpt, "SummaryVersionCreated should populate summary_excerpt")
	assert.Contains(t, homeItemsPort.upserted[1].SummaryExcerpt, "This is a detailed summary")
}

func TestKnowledgeProjectorJob_ArticleCreated_UpdatesTodayDigest(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	articleID := uuid.New()
	payload, _ := json.Marshal(articleCreatedPayload{
		ArticleID:   articleID.String(),
		Title:       "Test Article",
		PublishedAt: "2026-03-17T10:00:00Z",
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, EventType: domain.EventArticleCreated, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: payload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, digestPort.upserted, 1, "ArticleCreated should update today digest")
	assert.Equal(t, 1, digestPort.upserted[0].NewArticles)
}

func TestKnowledgeProjectorJob_SummaryVersionCreated_UpdatesTodayDigest(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	articleID := uuid.New()
	svID := uuid.New()
	summaryPayload, _ := json.Marshal(summaryVersionPayload{
		SummaryVersionID: svID.String(),
		ArticleID:        articleID.String(),
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, EventType: domain.EventSummaryVersionCreated, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: summaryPayload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}
	summaryVersionPort := &mockSummaryVersionPort{
		sv: domain.SummaryVersion{
			SummaryVersionID: svID,
			ArticleID:        articleID,
			SummaryText:      "Summary text here.",
		},
	}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, summaryVersionPort, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, digestPort.upserted, 1, "SummaryVersionCreated should update today digest")
	assert.Equal(t, 1, digestPort.upserted[0].SummarizedArticles)
}

func TestKnowledgeProjectorJob_HomeItemOpened_CreatesRecallCandidate(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	userID := uuid.New()
	openedPayload, _ := json.Marshal(homeItemOpenedPayload{
		ItemKey: "article:" + uuid.New().String(),
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, UserID: &userID, EventType: domain.EventHomeItemOpened, AggregateType: domain.AggregateArticle, AggregateID: "a1", Payload: openedPayload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}
	recallPort := &mockRecallCandidatePort{}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, recallPort)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, recallPort.upserted, 1, "HomeItemOpened should create recall candidate")
	assert.Equal(t, 0.5, recallPort.upserted[0].RecallScore)
}

func TestKnowledgeProjectorJob_UsesActiveVersion(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	articleID := uuid.New()
	payload, _ := json.Marshal(articleCreatedPayload{
		ArticleID: articleID.String(),
		Title:     "Test Article V2",
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, EventType: domain.EventArticleCreated, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: payload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}
	versionPort := &mockActiveVersionPort{
		version: &domain.KnowledgeProjectionVersion{Version: 2, Status: "active"},
	}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, versionPort, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)
	assert.Equal(t, 2, homeItemsPort.upserted[0].ProjectionVersion)
}
