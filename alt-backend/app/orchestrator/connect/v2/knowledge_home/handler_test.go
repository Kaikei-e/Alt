package knowledge_home

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/orchestrator/port/recall_candidate_port"
	"alt/orchestrator/usecase/get_knowledge_home_usecase"
	"alt/orchestrator/usecase/track_home_action_usecase"
	"alt/orchestrator/usecase/track_home_seen_usecase"
	"alt/utils/logger"
)

type mockRecallCandidatesPort struct {
	candidates []domain.RecallCandidate
	err        error
}

var _ recall_candidate_port.GetRecallCandidatesPort = (*mockRecallCandidatesPort)(nil)

func (m *mockRecallCandidatesPort) GetRecallCandidates(_ context.Context, _ uuid.UUID, _ int) ([]domain.RecallCandidate, error) {
	return m.candidates, m.err
}

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

func (m *mockKnowledgeEventPort) AppendKnowledgeEvent(_ context.Context, _ domain.KnowledgeEvent) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	return 1, nil
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

type mockLatestSeqPort struct {
	seq int64
	err error
}

func (m *mockLatestSeqPort) GetLatestKnowledgeEventSeqForUser(_ context.Context, _, _ uuid.UUID) (int64, error) {
	return m.seq, m.err
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

	getHomeUsecase := get_knowledge_home_usecase.NewGetKnowledgeHomeUsecase(homePort, digestPort, nil, nil, nil, nil)
	trackSeenUsecase := track_home_seen_usecase.NewTrackHomeSeenUsecase(userEventPort, flagPort)
	trackActionUsecase := track_home_action_usecase.NewTrackHomeActionUsecase(userEventPort, knowledgeEventPort, flagPort, nil, nil, nil, nil)

	handler := NewHandler(
		getHomeUsecase, trackSeenUsecase, trackActionUsecase,
		nil, nil, nil, // recall: rail, snooze, dismiss
		nil, nil, nil, nil, nil, // lens: create, update, list, select, archive
		nil, // eventsPort
		nil, // eventsForUserPort
		nil, // lensVisibilityPort
		nil, // resolveLensPort
		flagPort,
		nil, // metrics
		slog.Default(),
	)
	return handler, homePort, digestPort
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
			URL:          "https://example.com/article",
			WhyReasons:   []domain.WhyReason{{Code: "new_unread"}},
		}
		proto := convertHomeItemToProto(item)
		assert.Equal(t, "https://example.com/article", proto.Url)
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
			URL:          "",
			WhyReasons:   []domain.WhyReason{{Code: "new_unread"}},
		}
		proto := convertHomeItemToProto(item)
		assert.Empty(t, proto.Url)
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
		assert.Empty(t, proto.Url)
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
