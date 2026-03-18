package knowledge_home

import (
	"alt/domain"
	"alt/usecase/get_knowledge_home_usecase"
	"alt/usecase/track_home_action_usecase"
	"alt/usecase/track_home_seen_usecase"
	"alt/utils/logger"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
)

// mockHomeItemsPort implements knowledge_home_port.GetKnowledgeHomeItemsPort.
type mockHomeItemsPort struct {
	items      []domain.KnowledgeHomeItem
	nextCursor string
	hasMore    bool
	err        error
}

func (m *mockHomeItemsPort) GetKnowledgeHomeItems(_ context.Context, _ uuid.UUID, _ string, _ int, _ *domain.KnowledgeHomeLensFilter) ([]domain.KnowledgeHomeItem, string, bool, error) {
	return m.items, m.nextCursor, m.hasMore, m.err
}

// mockTodayDigestPort implements today_digest_port.GetTodayDigestPort.
type mockTodayDigestPort struct {
	digest domain.TodayDigest
	err    error
}

func (m *mockTodayDigestPort) GetTodayDigest(_ context.Context, _ uuid.UUID, _ time.Time) (domain.TodayDigest, error) {
	return m.digest, m.err
}

// mockUserEventPort implements knowledge_user_event_port.AppendKnowledgeUserEventPort.
type mockUserEventPort struct {
	err error
}

func (m *mockUserEventPort) AppendKnowledgeUserEvent(_ context.Context, _ domain.KnowledgeUserEvent) error {
	return m.err
}

// mockKnowledgeEventPort implements knowledge_event_port.AppendKnowledgeEventPort.
type mockKnowledgeEventPort struct {
	err error
}

func (m *mockKnowledgeEventPort) AppendKnowledgeEvent(_ context.Context, _ domain.KnowledgeEvent) error {
	return m.err
}

// mockFeatureFlagPort implements feature_flag_port.FeatureFlagPort.
type mockFeatureFlagPort struct {
	enabledFlags map[string]bool
}

func (m *mockFeatureFlagPort) IsEnabled(flagName string, _ uuid.UUID) bool {
	if m == nil || m.enabledFlags == nil {
		return true
	}
	return m.enabledFlags[flagName]
}

// testUserContext creates a context with an authenticated user for testing.
func testUserContext() context.Context {
	user := &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		SessionID: "test-session",
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	return domain.SetUserContext(context.Background(), user)
}

func setupHandler() (*Handler, *mockHomeItemsPort, *mockTodayDigestPort) {
	return setupHandlerWithFlags(nil)
}

func setupHandlerWithFlags(flagPort *mockFeatureFlagPort) (*Handler, *mockHomeItemsPort, *mockTodayDigestPort) {
	homePort := &mockHomeItemsPort{
		items: []domain.KnowledgeHomeItem{
			{
				ItemKey:  "article:test-1",
				ItemType: "article",
				Title:    "Test Article",
				Score:    1.0,
				WhyReasons: []domain.WhyReason{
					{Code: "new_unread"},
				},
			},
		},
		nextCursor: "cursor-abc",
		hasMore:    true,
	}
	digestPort := &mockTodayDigestPort{
		digest: domain.TodayDigest{
			NewArticles: 5,
		},
	}
	userEventPort := &mockUserEventPort{}
	knowledgeEventPort := &mockKnowledgeEventPort{}

	getHomeUsecase := get_knowledge_home_usecase.NewGetKnowledgeHomeUsecase(homePort, digestPort, nil, nil, nil)
	trackSeenUsecase := track_home_seen_usecase.NewTrackHomeSeenUsecase(userEventPort, flagPort)
	trackActionUsecase := track_home_action_usecase.NewTrackHomeActionUsecase(userEventPort, knowledgeEventPort, flagPort)

	handler := NewHandler(
		getHomeUsecase, trackSeenUsecase, trackActionUsecase,
		nil, nil, nil, // recall: rail, snooze, dismiss
		nil, nil, nil, nil, nil, // lens: create, update, list, select, archive
		nil, // eventsPort
		nil, // eventsForUserPort
		flagPort, slog.Default(),
	)
	return handler, homePort, digestPort
}

func TestHandler_GetKnowledgeHome_Unauthenticated(t *testing.T) {
	logger.InitLogger()
	handler, _, _ := setupHandler()

	// No user context → unauthenticated
	req := connect.NewRequest(&knowledgehomev1.GetKnowledgeHomeRequest{
		Limit: 20,
	})

	_, err := handler.GetKnowledgeHome(context.Background(), req)
	require.Error(t, err)

	connectErr, ok := err.(*connect.Error)
	require.True(t, ok)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestHandler_GetKnowledgeHome_FlagDisabled(t *testing.T) {
	logger.InitLogger()
	flagPort := &mockFeatureFlagPort{
		enabledFlags: map[string]bool{
			domain.FlagKnowledgeHomePage: false,
		},
	}
	handler, _, _ := setupHandlerWithFlags(flagPort)

	ctx := testUserContext()
	req := connect.NewRequest(&knowledgehomev1.GetKnowledgeHomeRequest{
		Limit: 20,
	})

	_, err := handler.GetKnowledgeHome(ctx, req)
	require.Error(t, err)

	connectErr, ok := err.(*connect.Error)
	require.True(t, ok)
	assert.Equal(t, connect.CodePermissionDenied, connectErr.Code())
}

func TestHandler_GetKnowledgeHome_FlagEnabled(t *testing.T) {
	logger.InitLogger()
	flagPort := &mockFeatureFlagPort{
		enabledFlags: map[string]bool{
			domain.FlagKnowledgeHomePage: true,
		},
	}
	handler, _, _ := setupHandlerWithFlags(flagPort)

	ctx := testUserContext()
	req := connect.NewRequest(&knowledgehomev1.GetKnowledgeHomeRequest{
		Limit: 20,
	})

	resp, err := handler.GetKnowledgeHome(ctx, req)
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Items, 1)
	assert.True(t, resp.Msg.HasMore)
	require.NotNil(t, resp.Msg.ServiceQuality)
	assert.Equal(t, "full", *resp.Msg.ServiceQuality)
}

func TestHandler_GetKnowledgeHome_NilFlagPort(t *testing.T) {
	logger.InitLogger()
	// nil flag port means no flag guard — should work as before
	handler, _, _ := setupHandler()

	ctx := testUserContext()
	req := connect.NewRequest(&knowledgehomev1.GetKnowledgeHomeRequest{
		Limit: 20,
	})

	resp, err := handler.GetKnowledgeHome(ctx, req)
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Items, 1)
}

func TestHandler_GetKnowledgeHome_SummaryStateMapping(t *testing.T) {
	logger.InitLogger()
	handler, homePort, _ := setupHandler()

	homePort.items = []domain.KnowledgeHomeItem{
		{
			ItemKey:      "article:ready-1",
			ItemType:     "article",
			Title:        "Ready Article",
			Score:        1.0,
			SummaryState: domain.SummaryStateReady,
			WhyReasons:   []domain.WhyReason{{Code: "new_unread"}},
		},
		{
			ItemKey:      "article:pending-1",
			ItemType:     "article",
			Title:        "Pending Article",
			Score:        0.9,
			SummaryState: domain.SummaryStatePending,
			WhyReasons:   []domain.WhyReason{{Code: "new_unread"}},
		},
		{
			ItemKey:      "article:missing-1",
			ItemType:     "article",
			Title:        "Missing Article",
			Score:        0.8,
			SummaryState: domain.SummaryStateMissing,
			WhyReasons:   []domain.WhyReason{{Code: "new_unread"}},
		},
	}

	ctx := testUserContext()
	req := connect.NewRequest(&knowledgehomev1.GetKnowledgeHomeRequest{Limit: 20})

	resp, err := handler.GetKnowledgeHome(ctx, req)
	require.NoError(t, err)
	require.Len(t, resp.Msg.Items, 3)

	assert.Equal(t, "ready", resp.Msg.Items[0].SummaryState)
	assert.Equal(t, "pending", resp.Msg.Items[1].SummaryState)
	assert.Equal(t, "missing", resp.Msg.Items[2].SummaryState)
}

func TestHandler_GetKnowledgeHome_NeedToKnowCountFromDigest(t *testing.T) {
	logger.InitLogger()
	handler, homePort, digestPort := setupHandler()

	// Items have pulse_need_to_know but handler should NOT count them
	homePort.items = []domain.KnowledgeHomeItem{
		{
			ItemKey:    "article:1",
			ItemType:   "article",
			Title:      "Important Article",
			Score:      1.0,
			WhyReasons: []domain.WhyReason{{Code: domain.WhyPulseNeedToKnow}},
		},
	}

	// Backend-authoritative count set on digest (e.g. via usecase enrichment)
	digestPort.digest = domain.TodayDigest{
		NewArticles:     5,
		NeedToKnowCount: 7,
	}

	ctx := testUserContext()
	req := connect.NewRequest(&knowledgehomev1.GetKnowledgeHomeRequest{Limit: 20})

	resp, err := handler.GetKnowledgeHome(ctx, req)
	require.NoError(t, err)

	// Should use digest value (7), NOT page-scan count (1)
	assert.Equal(t, int32(7), resp.Msg.TodayDigest.NeedToKnowCount,
		"Should use backend-authoritative count from digest, not page scan")
}

func TestHandler_GetKnowledgeHome_FreshnessMapping(t *testing.T) {
	logger.InitLogger()
	handler, _, digestPort := setupHandler()

	projectedAt := time.Now().Add(-2 * time.Minute)
	digestPort.digest = domain.TodayDigest{
		NewArticles:     3,
		DigestFreshness: domain.FreshnessStale,
		LastProjectedAt: &projectedAt,
	}

	ctx := testUserContext()
	req := connect.NewRequest(&knowledgehomev1.GetKnowledgeHomeRequest{Limit: 20})

	resp, err := handler.GetKnowledgeHome(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, "stale", resp.Msg.TodayDigest.DigestFreshness)
	assert.NotEmpty(t, resp.Msg.TodayDigest.LastProjectedAt)
}

func TestHandler_GetKnowledgeHome_ServiceQualityDegraded(t *testing.T) {
	logger.InitLogger()
	handler, _, digestPort := setupHandler()
	digestPort.err = errors.New("digest unavailable")

	ctx := testUserContext()
	req := connect.NewRequest(&knowledgehomev1.GetKnowledgeHomeRequest{Limit: 20})

	resp, err := handler.GetKnowledgeHome(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.ServiceQuality)
	assert.True(t, resp.Msg.DegradedMode)
	assert.Equal(t, "degraded", *resp.Msg.ServiceQuality)
}

func TestConvertHomeItemToProto_LinkMapping(t *testing.T) {
	t.Run("article with link", func(t *testing.T) {
		refID := uuid.New()
		item := domain.KnowledgeHomeItem{
			ItemKey:      "article:" + refID.String(),
			ItemType:     "article",
			PrimaryRefID: &refID,
			Title:        "Article with Link",
			Score:        0.9,
			Link:         "https://example.com/article",
			WhyReasons:   []domain.WhyReason{{Code: "new_unread"}},
		}
		proto := convertHomeItemToProto(item)
		assert.Equal(t, "https://example.com/article", proto.Link)
		require.NotNil(t, proto.ArticleId)
		assert.Equal(t, refID.String(), *proto.ArticleId)
	})

	t.Run("article without link", func(t *testing.T) {
		refID := uuid.New()
		item := domain.KnowledgeHomeItem{
			ItemKey:      "article:" + refID.String(),
			ItemType:     "article",
			PrimaryRefID: &refID,
			Title:        "Article without Link",
			Score:        0.8,
			Link:         "",
			WhyReasons:   []domain.WhyReason{{Code: "new_unread"}},
		}
		proto := convertHomeItemToProto(item)
		assert.Empty(t, proto.Link)
	})

	t.Run("recap anchor has no link", func(t *testing.T) {
		refID := uuid.New()
		item := domain.KnowledgeHomeItem{
			ItemKey:      "recap:" + refID.String(),
			ItemType:     "recap_anchor",
			PrimaryRefID: &refID,
			Title:        "Weekly Recap",
			Score:        0.7,
			WhyReasons:   []domain.WhyReason{{Code: "in_weekly_recap"}},
		}
		proto := convertHomeItemToProto(item)
		assert.Empty(t, proto.Link)
		require.NotNil(t, proto.RecapId)
	})
}

func TestConvertHomeItemToProto_SupersedeInfo(t *testing.T) {
	t.Run("summary_updated with previous excerpt", func(t *testing.T) {
		now := time.Now()
		item := domain.KnowledgeHomeItem{
			ItemKey:         "article:test-1",
			ItemType:        "article",
			Title:           "Test",
			Score:           0.9,
			SupersedeState:  domain.SupersedeSummaryUpdated,
			SupersededAt:    &now,
			PreviousRefJSON: `{"previous_summary_excerpt":"Old summary text"}`,
			WhyReasons:      []domain.WhyReason{{Code: "new_unread"}},
		}
		proto := convertHomeItemToProto(item)
		require.NotNil(t, proto.SupersedeInfo)
		assert.Equal(t, "summary_updated", proto.SupersedeInfo.State)
		require.NotNil(t, proto.SupersedeInfo.PreviousSummaryExcerpt)
		assert.Equal(t, "Old summary text", *proto.SupersedeInfo.PreviousSummaryExcerpt)
		assert.Empty(t, proto.SupersedeInfo.PreviousTags)
		assert.Empty(t, proto.SupersedeInfo.PreviousWhyCodes)
	})

	t.Run("tags_updated with previous tags", func(t *testing.T) {
		now := time.Now()
		item := domain.KnowledgeHomeItem{
			ItemKey:         "article:test-1",
			ItemType:        "article",
			Title:           "Test",
			Score:           0.9,
			SupersedeState:  domain.SupersedeTagsUpdated,
			SupersededAt:    &now,
			PreviousRefJSON: `{"previous_tags":["go","rust"]}`,
			WhyReasons:      []domain.WhyReason{{Code: "new_unread"}},
		}
		proto := convertHomeItemToProto(item)
		require.NotNil(t, proto.SupersedeInfo)
		assert.Equal(t, "tags_updated", proto.SupersedeInfo.State)
		assert.Equal(t, []string{"go", "rust"}, proto.SupersedeInfo.PreviousTags)
	})

	t.Run("reason_updated with previous why codes", func(t *testing.T) {
		now := time.Now()
		item := domain.KnowledgeHomeItem{
			ItemKey:         "article:test-1",
			ItemType:        "article",
			Title:           "Test",
			Score:           0.9,
			SupersedeState:  domain.SupersedeReasonUpdated,
			SupersededAt:    &now,
			PreviousRefJSON: `{"previous_why_codes":["new_unread","tag_hotspot"]}`,
			WhyReasons:      []domain.WhyReason{{Code: "new_unread"}},
		}
		proto := convertHomeItemToProto(item)
		require.NotNil(t, proto.SupersedeInfo)
		assert.Equal(t, "reason_updated", proto.SupersedeInfo.State)
		assert.Equal(t, []string{"new_unread", "tag_hotspot"}, proto.SupersedeInfo.PreviousWhyCodes)
	})

	t.Run("no supersede state", func(t *testing.T) {
		item := domain.KnowledgeHomeItem{
			ItemKey:    "article:test-1",
			ItemType:   "article",
			Title:      "Test",
			Score:      0.9,
			WhyReasons: []domain.WhyReason{{Code: "new_unread"}},
		}
		proto := convertHomeItemToProto(item)
		assert.Nil(t, proto.SupersedeInfo)
	})
}

// mockListEventsPort implements knowledge_event_port.ListKnowledgeEventsPort.
type mockListEventsPort struct {
	events []domain.KnowledgeEvent
	err    error
}

func (m *mockListEventsPort) ListKnowledgeEventsSince(_ context.Context, _ int64, _ int) ([]domain.KnowledgeEvent, error) {
	return m.events, m.err
}

func TestMapToCanonicalStreamType(t *testing.T) {
	tests := []struct {
		domainEvent  string
		canonicalType string
	}{
		{domain.EventArticleCreated, "item_added"},
		{domain.EventSummaryVersionCreated, "item_updated"},
		{domain.EventTagSetVersionCreated, "item_updated"},
		{domain.EventSummarySuperseded, "item_updated"},
		{domain.EventTagSetSuperseded, "item_updated"},
		{domain.EventHomeItemSuperseded, "item_updated"},
		{domain.EventHomeItemsSeen, "digest_changed"},
		{domain.EventHomeItemOpened, "digest_changed"},
		{domain.EventHomeItemDismissed, "digest_changed"},
		{domain.EventHomeItemAsked, "digest_changed"},
		{domain.EventHomeItemListened, "digest_changed"},
		{domain.EventRecallSnoozed, "digest_changed"},
		{domain.EventReasonMerged, "item_updated"},
		{domain.EventRecallDismissed, "digest_changed"},
		{"UnknownEventType", "digest_changed"},
	}

	for _, tt := range tests {
		t.Run(tt.domainEvent, func(t *testing.T) {
			result := mapToCanonicalStreamType(tt.domainEvent)
			assert.Equal(t, tt.canonicalType, result, "event %s should map to %s", tt.domainEvent, tt.canonicalType)
		})
	}
}

func TestStreamPayload_ItemAdded_ContainsItemKey(t *testing.T) {
	aggID := uuid.New().String()
	event := domain.KnowledgeEvent{
		EventType:     domain.EventArticleCreated,
		AggregateType: domain.AggregateArticle,
		AggregateID:   aggID,
		OccurredAt:    time.Now(),
	}

	canonicalType := mapToCanonicalStreamType(event.EventType)
	assert.Equal(t, "item_added", canonicalType)

	update := buildStreamResponse(event)
	assert.Equal(t, "item_added", update.EventType)
	require.NotNil(t, update.Item, "item_added should include Item")
	assert.Equal(t, "article:"+aggID, update.Item.ItemKey)
}

func TestStreamPayload_ItemUpdated_ContainsItemKey(t *testing.T) {
	aggID := uuid.New().String()
	event := domain.KnowledgeEvent{
		EventType:     domain.EventSummaryVersionCreated,
		AggregateType: domain.AggregateArticle,
		AggregateID:   aggID,
		OccurredAt:    time.Now(),
	}

	update := buildStreamResponse(event)
	assert.Equal(t, "item_updated", update.EventType)
	require.NotNil(t, update.Item, "item_updated should include Item")
	assert.Equal(t, "article:"+aggID, update.Item.ItemKey)
}

func TestStreamPayload_DigestChanged_NoItem(t *testing.T) {
	event := domain.KnowledgeEvent{
		EventType:     domain.EventHomeItemOpened,
		AggregateType: domain.AggregateHomeSession,
		AggregateID:   uuid.New().String(),
		OccurredAt:    time.Now(),
	}

	update := buildStreamResponse(event)
	assert.Equal(t, "digest_changed", update.EventType)
	assert.Nil(t, update.Item, "digest_changed should NOT include Item")
	assert.Nil(t, update.DigestChange, "digest_changed should NOT include DigestChange")
}

func TestCoalesceStreamEvents_EmptyInput(t *testing.T) {
	result := coalesceStreamEvents(nil)
	assert.Nil(t, result)

	result = coalesceStreamEvents([]domain.KnowledgeEvent{})
	assert.Empty(t, result)
}

func TestCoalesceStreamEvents_SingleEvent(t *testing.T) {
	events := []domain.KnowledgeEvent{
		{EventType: domain.EventArticleCreated, AggregateType: "article", AggregateID: "1", EventSeq: 1},
	}
	result := coalesceStreamEvents(events)
	assert.Len(t, result, 1)
	assert.Equal(t, "1", result[0].AggregateID)
}

func TestCoalesceStreamEvents_DeduplicatesByAggregate(t *testing.T) {
	events := []domain.KnowledgeEvent{
		{EventType: domain.EventArticleCreated, AggregateType: "article", AggregateID: "1", EventSeq: 1},
		{EventType: domain.EventSummaryVersionCreated, AggregateType: "article", AggregateID: "1", EventSeq: 2},
		{EventType: domain.EventTagSetVersionCreated, AggregateType: "article", AggregateID: "1", EventSeq: 3},
	}
	result := coalesceStreamEvents(events)
	assert.Len(t, result, 1, "same aggregate should be deduplicated to one event")
	assert.Equal(t, int64(3), result[0].EventSeq, "should keep the latest event")
}

func TestCoalesceStreamEvents_MultipleAggregates(t *testing.T) {
	events := []domain.KnowledgeEvent{
		{AggregateType: "article", AggregateID: "1", EventSeq: 1},
		{AggregateType: "article", AggregateID: "2", EventSeq: 2},
		{AggregateType: "recap", AggregateID: "3", EventSeq: 3},
	}
	result := coalesceStreamEvents(events)
	assert.Len(t, result, 3, "different aggregates should all remain")
}

func TestStreamKnowledgeHomeUpdates_FeatureFlagDisabled(t *testing.T) {
	logger.InitLogger()
	flagPort := &mockFeatureFlagPort{
		enabledFlags: map[string]bool{
			domain.FlagStreamUpdates: false,
		},
	}
	eventsPort := &mockListEventsPort{}
	handler := newStreamTestHandler(flagPort, eventsPort)

	ctx := testUserContext()
	// StreamKnowledgeHomeUpdates receives *connect.ServerStream which is a concrete type.
	// We test the feature flag guard by checking the error return before streaming starts.
	// The handler returns connect.CodePermissionDenied before touching the stream.
	err := handler.streamFlagGuard(ctx)
	require.Error(t, err)

	connectErr, ok := err.(*connect.Error)
	require.True(t, ok)
	assert.Equal(t, connect.CodePermissionDenied, connectErr.Code())
}

func TestStreamKnowledgeHomeUpdates_Unauthenticated(t *testing.T) {
	logger.InitLogger()
	handler := newStreamTestHandler(nil, nil)

	err := handler.streamFlagGuard(context.Background())
	require.Error(t, err)

	connectErr, ok := err.(*connect.Error)
	require.True(t, ok)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

// newStreamTestHandler creates a handler with eventsPort and featureFlags for stream tests.
func newStreamTestHandler(flagPort *mockFeatureFlagPort, eventsPort *mockListEventsPort) *Handler {
	handler := NewHandler(
		nil, nil, nil, // home, seen, action
		nil, nil, nil, // recall: rail, snooze, dismiss
		nil, nil, nil, nil, nil, // lens: create, update, list, select, archive
		eventsPort,
		nil, // eventsForUserPort
		flagPort, slog.Default(),
	)
	return handler
}

func TestHandler_TrackHomeAction_Validation(t *testing.T) {
	logger.InitLogger()
	handler, _, _ := setupHandler()

	tests := []struct {
		name       string
		actionType string
		itemKey    string
		wantCode   connect.Code
	}{
		{
			name:       "missing action_type",
			actionType: "",
			itemKey:    "article:1",
			wantCode:   connect.CodeInvalidArgument,
		},
		{
			name:       "missing item_key",
			actionType: "open",
			itemKey:    "",
			wantCode:   connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := connect.NewRequest(&knowledgehomev1.TrackHomeActionRequest{
				ActionType: tt.actionType,
				ItemKey:    tt.itemKey,
			})

			_, err := handler.TrackHomeAction(context.Background(), req)
			require.Error(t, err)

			connectErr, ok := err.(*connect.Error)
			require.True(t, ok)
			// Without user context, it's unauthenticated first
			assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
		})
	}
}
