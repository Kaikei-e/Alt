package job

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockEventsPort struct {
	events []domain.KnowledgeEvent
	err    error
	calls  int
}

func (m *mockEventsPort) ListKnowledgeEventsSince(_ context.Context, since int64, limit int) ([]domain.KnowledgeEvent, error) {
	m.calls++
	var result []domain.KnowledgeEvent
	for _, e := range m.events {
		if e.EventSeq > since {
			result = append(result, e)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, m.err
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
	upserted  []domain.KnowledgeHomeItem
	dismissed []struct {
		userID  uuid.UUID
		itemKey string
	}
	err   error
	items map[string]domain.KnowledgeHomeItem
}

func (m *mockHomeItemsPort) UpsertKnowledgeHomeItem(_ context.Context, item domain.KnowledgeHomeItem) error {
	if m.err != nil {
		return m.err
	}
	m.upserted = append(m.upserted, item)
	if m.items == nil {
		m.items = make(map[string]domain.KnowledgeHomeItem)
	}
	existing, ok := m.items[item.ItemKey]
	if ok {
		if existing.DismissedAt != nil && item.DismissedAt == nil {
			item.DismissedAt = existing.DismissedAt
		}
		existing.WhyReasons = mergeWhyReasons(item.WhyReasons, existing.WhyReasons)
		if item.Title != "" {
			existing.Title = item.Title
		}
		if item.SummaryExcerpt != "" {
			existing.SummaryExcerpt = item.SummaryExcerpt
		}
		if len(item.Tags) > 0 {
			existing.Tags = item.Tags
		}
		if item.SummaryState == domain.SummaryStateReady || (item.SummaryState != "" && item.SummaryState != domain.SummaryStateMissing) {
			existing.SummaryState = item.SummaryState
		}
		existing.DismissedAt = item.DismissedAt
		m.items[item.ItemKey] = existing
		return nil
	}
	m.items[item.ItemKey] = item
	return nil
}

func (m *mockHomeItemsPort) DismissKnowledgeHomeItem(_ context.Context, userID uuid.UUID, itemKey string, _ int, dismissedAt time.Time) error {
	if m.err != nil {
		return m.err
	}
	m.dismissed = append(m.dismissed, struct {
		userID  uuid.UUID
		itemKey string
	}{userID, itemKey})
	if m.items != nil {
		existing, ok := m.items[itemKey]
		if ok {
			existing.DismissedAt = &dismissedAt
			m.items[itemKey] = existing
		}
	}
	return nil
}

func (m *mockHomeItemsPort) ClearSupersedeState(_ context.Context, _ uuid.UUID, _ string, _ int) error {
	return m.err
}

func mergeWhyReasons(newReasons []domain.WhyReason, existingReasons []domain.WhyReason) []domain.WhyReason {
	merged := make(map[string]domain.WhyReason, len(newReasons)+len(existingReasons))
	for _, reason := range existingReasons {
		merged[reason.Code] = reason
	}
	for _, reason := range newReasons {
		merged[reason.Code] = reason
	}
	result := make([]domain.WhyReason, 0, len(merged))
	for _, reason := range merged {
		result = append(result, reason)
	}
	return result
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

type mockSummaryVersionPort struct {
	sv       domain.SummaryVersion
	err      error
	calledID uuid.UUID // tracks the ID passed to GetSummaryVersionByID
}

func (m *mockSummaryVersionPort) GetSummaryVersionByID(_ context.Context, summaryVersionID uuid.UUID) (domain.SummaryVersion, error) {
	m.calledID = summaryVersionID
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

type mockTagSetVersionPort struct {
	tsv domain.TagSetVersion
	err error
}

func (m *mockTagSetVersionPort) GetTagSetVersionByID(_ context.Context, _ uuid.UUID) (domain.TagSetVersion, error) {
	return m.tsv, m.err
}

func TestKnowledgeProjectorJob_NoEvents(t *testing.T) {
	logger.InitLogger()

	eventsPort := &mockEventsPort{events: nil}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, nil)
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

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, nil)
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

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	assert.Equal(t, int64(10), checkpointPort.updatedSeq) // Checkpoint advances to max seq
	assert.Len(t, homeItemsPort.upserted, 2)
}

func TestKnowledgeProjectorJob_DrainsMultipleBatchesWithinSingleRun(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	events := make([]domain.KnowledgeEvent, 0, 150)
	for i := 0; i < 150; i++ {
		payload, _ := json.Marshal(articleCreatedPayload{
			ArticleID: uuid.New().String(),
			Title:     "Article",
		})
		events = append(events, domain.KnowledgeEvent{
			EventID:       uuid.New(),
			EventSeq:      int64(i + 1),
			TenantID:      tenantID,
			EventType:     domain.EventArticleCreated,
			AggregateType: domain.AggregateArticle,
			AggregateID:   uuid.New().String(),
			Payload:       payload,
		})
	}

	eventsPort := &mockEventsPort{events: events}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	assert.Len(t, homeItemsPort.upserted, 150)
	assert.Equal(t, int64(150), checkpointPort.updatedSeq)
	assert.GreaterOrEqual(t, eventsPort.calls, 2)
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

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, summaryVersionPort, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 2)
	// Second upsert is the summary version event - should have excerpt
	assert.NotEmpty(t, homeItemsPort.upserted[1].SummaryExcerpt, "SummaryVersionCreated should populate summary_excerpt")
	assert.Contains(t, homeItemsPort.upserted[1].SummaryExcerpt, "This is a detailed summary")
}

func TestKnowledgeProjectorJob_SummaryVersionCreated_ReplaySafe(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	articleID := uuid.New()
	oldSvID := uuid.New()
	newSvID := uuid.New()

	// Replay an OLD event whose summary_version_id points to an older version.
	// The projector must use the event's summary_version_id, not "latest".
	summaryPayload, _ := json.Marshal(summaryVersionPayload{
		SummaryVersionID: oldSvID.String(),
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

	// Mock returns old version text — the projector must request oldSvID, not newSvID.
	summaryVersionPort := &mockSummaryVersionPort{
		sv: domain.SummaryVersion{
			SummaryVersionID: oldSvID,
			ArticleID:        articleID,
			SummaryText:      "Old version excerpt that should be used on replay.",
		},
	}
	_ = newSvID // newer version exists but must NOT be fetched

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, summaryVersionPort, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	// Verify the projector called GetSummaryVersionByID with the event's summary_version_id
	assert.Equal(t, oldSvID, summaryVersionPort.calledID, "projector must use event's summary_version_id, not latest")
	require.Len(t, homeItemsPort.upserted, 1)
	assert.Contains(t, homeItemsPort.upserted[0].SummaryExcerpt, "Old version excerpt")
}

func TestKnowledgeProjectorJob_TagSetVersionCreated_PreservesSummaryCompletedReason(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	articleID := uuid.New()
	summaryVersionID := uuid.New()
	tagSetVersionID := uuid.New()

	articlePayload, _ := json.Marshal(articleCreatedPayload{
		ArticleID:   articleID.String(),
		Title:       "Test Article",
		PublishedAt: "2026-03-17T10:00:00Z",
	})
	summaryPayload, _ := json.Marshal(summaryVersionPayload{
		SummaryVersionID: summaryVersionID.String(),
		ArticleID:        articleID.String(),
	})
	tagPayload, _ := json.Marshal(tagSetVersionPayload{
		TagSetVersionID: tagSetVersionID.String(),
		ArticleID:       articleID.String(),
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, EventType: domain.EventArticleCreated, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: articlePayload},
			{EventID: uuid.New(), EventSeq: 2, TenantID: tenantID, EventType: domain.EventSummaryVersionCreated, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: summaryPayload},
			{EventID: uuid.New(), EventSeq: 3, TenantID: tenantID, EventType: domain.EventTagSetVersionCreated, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: tagPayload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}
	summaryVersionPort := &mockSummaryVersionPort{
		sv: domain.SummaryVersion{
			SummaryVersionID: summaryVersionID,
			ArticleID:        articleID,
			SummaryText:      "This is a detailed summary of the test article covering important topics.",
		},
	}
	tagSetVersionPort := &mockTagSetVersionPort{
		tsv: domain.TagSetVersion{
			TagSetVersionID: tagSetVersionID,
			ArticleID:       articleID,
			TagsJSON:        json.RawMessage(`[{"name":"AI"},{"name":"ML"}]`),
		},
	}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, summaryVersionPort, nil, tagSetVersionPort)
	err := fn(context.Background())

	require.NoError(t, err)
	current, ok := homeItemsPort.items["article:"+articleID.String()]
	require.True(t, ok)
	assert.ElementsMatch(t, []domain.WhyReason{
		{Code: domain.WhyNewUnread},
		{Code: domain.WhySummaryCompleted},
		{Code: domain.WhyTagHotspot, Tag: "AI"},
	}, current.WhyReasons)
	assert.Equal(t, []string{"AI", "ML"}, current.Tags)
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

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, digestPort.upserted, 1, "ArticleCreated should update today digest")
	assert.Equal(t, 1, digestPort.upserted[0].NewArticles)
	assert.Equal(t, 1, digestPort.upserted[0].UnsummarizedArticles)
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

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, summaryVersionPort, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, digestPort.upserted, 1, "SummaryVersionCreated should update today digest")
	assert.Equal(t, 1, digestPort.upserted[0].SummarizedArticles)
	assert.Equal(t, -1, digestPort.upserted[0].UnsummarizedArticles)
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

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, recallPort, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, recallPort.upserted, 1, "HomeItemOpened should create recall candidate")
	assert.Equal(t, 0.5, recallPort.upserted[0].RecallScore)
}

func TestKnowledgeProjectorJob_HomeItemOpened_ReprojectSafe(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	userID := uuid.New()
	itemKey := "article:" + uuid.New().String()
	openedPayload, _ := json.Marshal(homeItemOpenedPayload{ItemKey: itemKey})

	// Event occurred 25 hours ago
	eventTime := time.Now().Add(-25 * time.Hour)
	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, UserID: &userID, EventType: domain.EventHomeItemOpened, AggregateType: domain.AggregateArticle, AggregateID: "a1", Payload: openedPayload, OccurredAt: eventTime},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}
	recallPort := &mockRecallCandidatePort{}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, recallPort, nil)
	err := fn(context.Background())

	require.NoError(t, err)

	// Recall candidate eligibleAt should be based on event time, not time.Now()
	require.Len(t, recallPort.upserted, 1)
	candidate := recallPort.upserted[0]
	expectedEligibleAt := eventTime.Add(24 * time.Hour)
	assert.WithinDuration(t, expectedEligibleAt, *candidate.FirstEligibleAt, time.Second, "eligibleAt should be event time + 24h, not now + 24h")
	assert.WithinDuration(t, expectedEligibleAt, *candidate.NextSuggestAt, time.Second, "nextSuggestAt should be event time + 24h")

	// LastInteractedAt should be event time, not time.Now()
	require.Len(t, homeItemsPort.upserted, 1)
	item := homeItemsPort.upserted[0]
	assert.WithinDuration(t, eventTime, *item.LastInteractedAt, time.Second, "LastInteractedAt should be event time")
}

func TestKnowledgeProjectorJob_HomeItemDismissed_DismissesReadModelItem(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	userID := uuid.New()
	itemKey := "article:" + uuid.New().String()
	dismissPayload, _ := json.Marshal(map[string]string{
		"item_key": itemKey,
	})

	homeItemsPort := &mockHomeItemsPort{
		items: map[string]domain.KnowledgeHomeItem{
			itemKey: {
				UserID:   userID,
				TenantID: tenantID,
				ItemKey:  itemKey,
				ItemType: domain.ItemArticle,
				Title:    "Dismiss me",
			},
		},
	}
	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, UserID: &userID, EventType: domain.EventHomeItemDismissed, AggregateType: domain.AggregateHomeSession, AggregateID: itemKey, Payload: dismissPayload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	digestPort := &mockDigestPort{}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.dismissed, 1)
	assert.Equal(t, userID, homeItemsPort.dismissed[0].userID)
	assert.Equal(t, itemKey, homeItemsPort.dismissed[0].itemKey)
	assert.Empty(t, homeItemsPort.upserted, "dismiss should not upsert a replacement row")
	assert.NotNil(t, homeItemsPort.items[itemKey].DismissedAt)
}

func TestKnowledgeProjectorJob_DismissedItemStaysDismissedAfterLaterUpsert(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	userID := uuid.New()
	articleID := uuid.New()
	itemKey := "article:" + articleID.String()
	dismissedAt := time.Date(2026, 3, 18, 14, 21, 33, 0, time.UTC)

	dismissPayload, _ := json.Marshal(map[string]string{
		"item_key": itemKey,
	})
	summaryPayload, _ := json.Marshal(summaryVersionPayload{
		SummaryVersionID: uuid.New().String(),
		ArticleID:        articleID.String(),
	})

	homeItemsPort := &mockHomeItemsPort{
		items: map[string]domain.KnowledgeHomeItem{
			itemKey: {
				UserID:   userID,
				TenantID: tenantID,
				ItemKey:  itemKey,
				ItemType: domain.ItemArticle,
				Title:    "Dismiss me",
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
				EventType:     domain.EventHomeItemDismissed,
				AggregateType: domain.AggregateHomeSession,
				AggregateID:   itemKey,
				Payload:       dismissPayload,
				OccurredAt:    dismissedAt,
			},
			{
				EventID:       uuid.New(),
				EventSeq:      2,
				TenantID:      tenantID,
				UserID:        &userID,
				EventType:     domain.EventSummaryVersionCreated,
				AggregateType: domain.AggregateArticle,
				AggregateID:   articleID.String(),
				Payload:       summaryPayload,
			},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	digestPort := &mockDigestPort{}
	summaryVersionPort := &mockSummaryVersionPort{
		sv: domain.SummaryVersion{
			SummaryText: "Refreshed summary",
		},
	}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, summaryVersionPort, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.NotNil(t, homeItemsPort.items[itemKey].DismissedAt)
	assert.True(t, homeItemsPort.items[itemKey].DismissedAt.Equal(dismissedAt))
	assert.Equal(t, "Refreshed summary", homeItemsPort.items[itemKey].SummaryExcerpt)
}

func TestKnowledgeProjectorJob_ArticleCreated_SetsSummaryStatePending(t *testing.T) {
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

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)
	assert.Equal(t, domain.SummaryStatePending, homeItemsPort.upserted[0].SummaryState, "ArticleCreated should set summary_state to pending")
}

func TestKnowledgeProjectorJob_SummaryVersionCreated_SetsSummaryStateReady(t *testing.T) {
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
			SummaryText:      "Complete summary text here.",
		},
	}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, summaryVersionPort, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)
	assert.Equal(t, domain.SummaryStateReady, homeItemsPort.upserted[0].SummaryState, "SummaryVersionCreated with text should set summary_state to ready")
}

func TestKnowledgeProjectorJob_SummaryVersionCreated_TruncatesUTF8Safely(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	articleID := uuid.New()
	svID := uuid.New()
	summaryPayload, _ := json.Marshal(summaryVersionPayload{
		SummaryVersionID: svID.String(),
		ArticleID:        articleID.String(),
	})

	longJapaneseSummary := "あ"
	for len([]rune(longJapaneseSummary)) <= maxExcerptLen {
		longJapaneseSummary += "要約"
	}

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
			SummaryText:      longJapaneseSummary,
		},
	}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, summaryVersionPort, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)
	assert.True(t, utf8.ValidString(homeItemsPort.upserted[0].SummaryExcerpt), "truncated excerpt must remain valid UTF-8")
	assert.LessOrEqual(t, len([]rune(homeItemsPort.upserted[0].SummaryExcerpt)), maxExcerptLen+1)
	assert.Equal(t, domain.SummaryStateReady, homeItemsPort.upserted[0].SummaryState)
}

func TestKnowledgeProjectorJob_SummaryVersionCreated_EmptySummary_SetsPending(t *testing.T) {
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
			SummaryText:      "", // Empty summary
		},
	}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, summaryVersionPort, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)
	assert.Equal(t, domain.SummaryStatePending, homeItemsPort.upserted[0].SummaryState, "SummaryVersionCreated with empty text should set summary_state to pending")
	assert.NotContains(t, homeItemsPort.upserted[0].WhyReasons, domain.WhyReason{Code: domain.WhySummaryCompleted}, "empty summaries must not add summary_completed")
	require.Len(t, digestPort.upserted, 1)
	assert.Equal(t, 0, digestPort.upserted[0].UnsummarizedArticles, "empty summaries must not reduce pending count")
}

func TestKnowledgeProjectorJob_SummaryVersionCreated_PortError_DoesNotAddSummaryCompletedReason(t *testing.T) {
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
		err: assert.AnError,
	}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, summaryVersionPort, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)
	assert.Equal(t, domain.SummaryStatePending, homeItemsPort.upserted[0].SummaryState)
	assert.NotContains(t, homeItemsPort.upserted[0].WhyReasons, domain.WhyReason{Code: domain.WhySummaryCompleted}, "failed summary fetch must not add summary_completed")
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

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, versionPort, nil, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)
	assert.Equal(t, 2, homeItemsPort.upserted[0].ProjectionVersion)
}

// ── Step C tests: TagSetVersionCreated tag projection ──

func TestKnowledgeProjectorJob_TagSetVersionCreated_ProjectsTags(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	userID := uuid.New()
	articleID := uuid.New()
	tsvID := uuid.New()

	tagsJSON, _ := json.Marshal([]tagItem{
		{Name: "golang", Confidence: 0.95},
		{Name: "architecture", Confidence: 0.8},
	})

	tagPayload, _ := json.Marshal(tagSetVersionPayload{
		TagSetVersionID: tsvID.String(),
		ArticleID:       articleID.String(),
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, UserID: &userID, EventType: domain.EventTagSetVersionCreated, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: tagPayload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}
	tagSetVersionPort := &mockTagSetVersionPort{
		tsv: domain.TagSetVersion{
			TagSetVersionID: tsvID,
			ArticleID:       articleID,
			UserID:          userID,
			Generator:       "tag-generator",
			TagsJSON:        tagsJSON,
		},
	}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, tagSetVersionPort)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)

	item := homeItemsPort.upserted[0]
	assert.Equal(t, "article:"+articleID.String(), item.ItemKey)
	assert.Equal(t, []string{"golang", "architecture"}, item.Tags, "tags should be projected from version")
	assert.Equal(t, "", item.Title, "title should be empty (preserved by merge-safe upsert)")
	assert.Equal(t, userID, item.UserID)
}

func TestKnowledgeProjectorJob_TagSetVersionCreated_AddsTagHotspot(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	articleID := uuid.New()
	tsvID := uuid.New()

	tagsJSON, _ := json.Marshal([]tagItem{
		{Name: "rust", Confidence: 0.9},
	})

	tagPayload, _ := json.Marshal(tagSetVersionPayload{
		TagSetVersionID: tsvID.String(),
		ArticleID:       articleID.String(),
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, EventType: domain.EventTagSetVersionCreated, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: tagPayload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}
	tagSetVersionPort := &mockTagSetVersionPort{
		tsv: domain.TagSetVersion{
			TagSetVersionID: tsvID,
			ArticleID:       articleID,
			TagsJSON:        tagsJSON,
		},
	}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, tagSetVersionPort)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)

	item := homeItemsPort.upserted[0]
	require.Len(t, item.WhyReasons, 2, "should have new_unread + tag_hotspot")
	assert.Equal(t, domain.WhyNewUnread, item.WhyReasons[0].Code)
	assert.Equal(t, domain.WhyTagHotspot, item.WhyReasons[1].Code)
	assert.Equal(t, "rust", item.WhyReasons[1].Tag, "tag_hotspot should reference first tag")
}

func TestKnowledgeProjectorJob_TagSetVersionCreated_DoesNotOverwriteTitleOrSummary(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	articleID := uuid.New()
	tsvID := uuid.New()

	tagsJSON, _ := json.Marshal([]tagItem{{Name: "ai", Confidence: 0.9}})

	tagPayload, _ := json.Marshal(tagSetVersionPayload{
		TagSetVersionID: tsvID.String(),
		ArticleID:       articleID.String(),
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, EventType: domain.EventTagSetVersionCreated, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: tagPayload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}
	tagSetVersionPort := &mockTagSetVersionPort{
		tsv: domain.TagSetVersion{TagSetVersionID: tsvID, ArticleID: articleID, TagsJSON: tagsJSON},
	}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, tagSetVersionPort)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)

	item := homeItemsPort.upserted[0]
	assert.Equal(t, "", item.Title, "title should be empty for merge-safe upsert")
	assert.Equal(t, "", item.SummaryExcerpt, "summary_excerpt should be empty for merge-safe upsert")
	assert.Equal(t, "", item.SummaryState, "summary_state should not be set by tag event")
}

func TestKnowledgeProjectorJob_TagSetVersionCreated_UsesEventVersionNotLatest(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	articleID := uuid.New()
	tsvID := uuid.New()

	// Mock returns a specific version matching the event's tag_set_version_id
	tagsJSON, _ := json.Marshal([]tagItem{{Name: "event-tag", Confidence: 0.9}})

	tagPayload, _ := json.Marshal(tagSetVersionPayload{
		TagSetVersionID: tsvID.String(),
		ArticleID:       articleID.String(),
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, EventType: domain.EventTagSetVersionCreated, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: tagPayload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}
	// Even if a newer version exists, the projector should use the event's version
	tagSetVersionPort := &mockTagSetVersionPort{
		tsv: domain.TagSetVersion{
			TagSetVersionID: tsvID,
			ArticleID:       articleID,
			TagsJSON:        tagsJSON,
		},
	}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, tagSetVersionPort)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)
	assert.Equal(t, []string{"event-tag"}, homeItemsPort.upserted[0].Tags, "should use tags from the event's specific version")
}

func TestKnowledgeProjectorJob_TagSetVersionCreated_NilPort_FallsBackGracefully(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	articleID := uuid.New()
	tsvID := uuid.New()

	tagPayload, _ := json.Marshal(tagSetVersionPayload{
		TagSetVersionID: tsvID.String(),
		ArticleID:       articleID.String(),
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, EventType: domain.EventTagSetVersionCreated, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: tagPayload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}

	// Pass nil tagSetVersionPort - should still work without tags
	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)
	assert.Empty(t, homeItemsPort.upserted[0].Tags, "tags should be empty when port is nil")
	assert.Len(t, homeItemsPort.upserted[0].WhyReasons, 1, "only new_unread, no tag_hotspot")
	assert.Equal(t, domain.WhyNewUnread, homeItemsPort.upserted[0].WhyReasons[0].Code)
}

// ── Supersede projection tests ──

type mockClearSupersedePort struct {
	cleared []struct {
		userID  uuid.UUID
		itemKey string
	}
	err error
}

func (m *mockClearSupersedePort) ClearSupersedeState(_ context.Context, userID uuid.UUID, itemKey string, _ int) error {
	if m.err != nil {
		return m.err
	}
	m.cleared = append(m.cleared, struct {
		userID  uuid.UUID
		itemKey string
	}{userID, itemKey})
	return nil
}

func TestKnowledgeProjectorJob_SummarySuperseded_ProjectsSupersedeState(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	userID := uuid.New()
	articleID := uuid.New()

	supersedePayload, _ := json.Marshal(map[string]string{
		"article_id":               articleID.String(),
		"new_summary_version_id":   uuid.New().String(),
		"old_summary_version_id":   uuid.New().String(),
		"previous_summary_excerpt": "Old summary text",
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, UserID: &userID, EventType: domain.EventSummarySuperseded, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: supersedePayload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)
	assert.Equal(t, domain.SupersedeSummaryUpdated, homeItemsPort.upserted[0].SupersedeState)
	assert.NotNil(t, homeItemsPort.upserted[0].SupersededAt)
	assert.Contains(t, homeItemsPort.upserted[0].PreviousRefJSON, "previous_summary_excerpt")
}

func TestKnowledgeProjectorJob_TagSetSuperseded_ProjectsSupersedeState(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	userID := uuid.New()
	articleID := uuid.New()

	supersedePayload, _ := json.Marshal(map[string]interface{}{
		"article_id":             articleID.String(),
		"new_tag_set_version_id": uuid.New().String(),
		"old_tag_set_version_id": uuid.New().String(),
		"previous_tags":          []string{"old-tag1", "old-tag2"},
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, UserID: &userID, EventType: domain.EventTagSetSuperseded, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: supersedePayload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)
	assert.Equal(t, domain.SupersedeTagsUpdated, homeItemsPort.upserted[0].SupersedeState)
	assert.Contains(t, homeItemsPort.upserted[0].PreviousRefJSON, "previous_tags")
}

// RED: TagSetSuperseded must set Tags to empty slice (not nil).
// nil serializes to "null" JSON which bypasses the merge-safe SQL guard
// and overwrites existing tags with null.
func TestKnowledgeProjectorJob_TagSetSuperseded_SetsEmptyTagsSlice(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	userID := uuid.New()
	articleID := uuid.New()

	supersedePayload, _ := json.Marshal(map[string]interface{}{
		"article_id":             articleID.String(),
		"new_tag_set_version_id": uuid.New().String(),
		"old_tag_set_version_id": uuid.New().String(),
		"previous_tags":          []string{"old-tag1", "old-tag2"},
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, UserID: &userID, EventType: domain.EventTagSetSuperseded, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: supersedePayload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)

	item := homeItemsPort.upserted[0]
	// Tags must be empty slice, not nil. nil → json "null" → overwrites existing tags.
	assert.NotNil(t, item.Tags, "Tags must not be nil (nil serializes to 'null' JSON)")
	assert.Empty(t, item.Tags, "Tags should be empty slice (no new tags in supersede)")
}

func TestKnowledgeProjectorJob_ReasonMerged_ProjectsSupersedeState(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	userID := uuid.New()
	articleID := uuid.New()

	reasonPayload, _ := json.Marshal(map[string]interface{}{
		"article_id":         articleID.String(),
		"item_key":           "article:" + articleID.String(),
		"added_codes":        []string{"in_weekly_recap"},
		"previous_why_codes": []string{"new_unread", "tag_hotspot"},
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, UserID: &userID, EventType: domain.EventReasonMerged, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: reasonPayload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, nil)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)
	assert.Equal(t, domain.SupersedeReasonUpdated, homeItemsPort.upserted[0].SupersedeState)
	assert.Contains(t, homeItemsPort.upserted[0].PreviousRefJSON, "previous_why_codes")
}

func TestKnowledgeProjectorJob_HomeItemOpened_ClearsSupersedeState(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	userID := uuid.New()
	itemKey := "article:" + uuid.New().String()

	openedPayload, _ := json.Marshal(homeItemOpenedPayload{
		ItemKey: itemKey,
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, UserID: &userID, EventType: domain.EventHomeItemOpened, AggregateType: domain.AggregateHomeSession, AggregateID: itemKey, Payload: openedPayload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}
	recallPort := &mockRecallCandidatePort{}
	clearPort := &mockClearSupersedePort{}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, recallPort, nil, clearPort)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, clearPort.cleared, 1, "HomeItemOpened should clear supersede state")
	assert.Equal(t, userID, clearPort.cleared[0].userID)
	assert.Equal(t, itemKey, clearPort.cleared[0].itemKey)
}

func TestKnowledgeProjectorJob_TagSetVersionCreated_UppercaseKeys(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()
	articleID := uuid.New()
	tsvID := uuid.New()

	// Simulate Go-default marshaling (uppercase keys, no json tags)
	type UpperTag struct {
		Name       string
		Confidence float32
	}
	tagsJSON, _ := json.Marshal([]UpperTag{
		{Name: "flowers", Confidence: 0.5},
		{Name: "history", Confidence: 0.5},
	})

	tagPayload, _ := json.Marshal(tagSetVersionPayload{
		TagSetVersionID: tsvID.String(),
		ArticleID:       articleID.String(),
	})

	eventsPort := &mockEventsPort{
		events: []domain.KnowledgeEvent{
			{EventID: uuid.New(), EventSeq: 1, TenantID: tenantID, EventType: domain.EventTagSetVersionCreated, AggregateType: domain.AggregateArticle, AggregateID: articleID.String(), Payload: tagPayload},
		},
	}
	checkpointPort := &mockCheckpointPort{lastSeq: 0}
	homeItemsPort := &mockHomeItemsPort{}
	digestPort := &mockDigestPort{}
	tagSetVersionPort := &mockTagSetVersionPort{
		tsv: domain.TagSetVersion{
			TagSetVersionID: tsvID,
			ArticleID:       articleID,
			TagsJSON:        tagsJSON,
		},
	}

	fn := KnowledgeProjectorJob(eventsPort, checkpointPort, checkpointPort, homeItemsPort, digestPort, nil, nil, nil, tagSetVersionPort)
	err := fn(context.Background())

	require.NoError(t, err)
	require.Len(t, homeItemsPort.upserted, 1)
	assert.Equal(t, []string{"flowers", "history"}, homeItemsPort.upserted[0].Tags, "should parse uppercase-keyed TagsJSON")
	assert.Len(t, homeItemsPort.upserted[0].WhyReasons, 2)
	assert.Equal(t, "flowers", homeItemsPort.upserted[0].WhyReasons[1].Tag)
}
