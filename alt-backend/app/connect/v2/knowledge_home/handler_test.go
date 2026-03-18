package knowledge_home

import (
	"alt/domain"
	"alt/usecase/get_knowledge_home_usecase"
	"alt/usecase/track_home_action_usecase"
	"alt/usecase/track_home_seen_usecase"
	"alt/utils/logger"
	"context"
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

func (m *mockHomeItemsPort) GetKnowledgeHomeItems(_ context.Context, _ uuid.UUID, _ string, _ int) ([]domain.KnowledgeHomeItem, string, bool, error) {
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

	getHomeUsecase := get_knowledge_home_usecase.NewGetKnowledgeHomeUsecase(homePort, digestPort)
	trackSeenUsecase := track_home_seen_usecase.NewTrackHomeSeenUsecase(userEventPort, flagPort)
	trackActionUsecase := track_home_action_usecase.NewTrackHomeActionUsecase(userEventPort, knowledgeEventPort, flagPort)

	handler := NewHandler(
		getHomeUsecase, trackSeenUsecase, trackActionUsecase,
		nil, nil, nil, // recall: rail, snooze, dismiss
		nil, nil, nil, nil, nil, // lens: create, update, list, select, archive
		nil, // eventsPort
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

func TestHandler_GetKnowledgeHome_NeedToKnowCount(t *testing.T) {
	logger.InitLogger()
	handler, homePort, _ := setupHandler()

	homePort.items = []domain.KnowledgeHomeItem{
		{
			ItemKey:  "article:1",
			ItemType: "article",
			Title:    "Important Article",
			Score:    1.0,
			WhyReasons: []domain.WhyReason{
				{Code: domain.WhyPulseNeedToKnow},
				{Code: domain.WhyNewUnread},
			},
		},
		{
			ItemKey:    "article:2",
			ItemType:   "article",
			Title:      "Regular Article",
			Score:      0.9,
			WhyReasons: []domain.WhyReason{{Code: domain.WhyNewUnread}},
		},
		{
			ItemKey:  "article:3",
			ItemType: "article",
			Title:    "Another Important Article",
			Score:    0.8,
			WhyReasons: []domain.WhyReason{
				{Code: domain.WhyPulseNeedToKnow},
			},
		},
	}

	ctx := testUserContext()
	req := connect.NewRequest(&knowledgehomev1.GetKnowledgeHomeRequest{Limit: 20})

	resp, err := handler.GetKnowledgeHome(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, int32(2), resp.Msg.TodayDigest.NeedToKnowCount, "Should count 2 items with pulse_need_to_know why code")
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
