package knowledge_home

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/orchestrator/usecase/get_knowledge_home_usecase"
	"alt/orchestrator/usecase/track_home_action_usecase"
	"alt/orchestrator/usecase/track_home_seen_usecase"
	"alt/utils/logger"
	altotel "alt/utils/otel"
)

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

func TestHandler_GetKnowledgeHome_WithMetrics_DoesNotPanic(t *testing.T) {
	logger.InitLogger()
	metrics, err := altotel.NewKnowledgeHomeMetrics()
	require.NoError(t, err)

	homePort := &mockHomeItemsPort{
		items: []domain.KnowledgeHomeItem{
			{ItemKey: "article:1", ItemType: "article", Title: "Test", Score: 1.0, WhyReasons: []domain.WhyReason{{Code: "new_unread"}}},
		},
	}
	digestPort := &mockTodayDigestPort{digest: domain.TodayDigest{NewArticles: 5}}
	userEventPort := &mockUserEventPort{}
	knowledgeEventPort := &mockKnowledgeEventPort{}

	getHomeUsecase := get_knowledge_home_usecase.NewGetKnowledgeHomeUsecase(homePort, digestPort, nil, nil, nil, nil)
	trackSeenUsecase := track_home_seen_usecase.NewTrackHomeSeenUsecase(userEventPort, nil)
	trackActionUsecase := track_home_action_usecase.NewTrackHomeActionUsecase(userEventPort, knowledgeEventPort, nil, nil, nil, nil, nil)

	handler := NewHandler(
		getHomeUsecase, trackSeenUsecase, trackActionUsecase,
		nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		metrics,
		slog.Default(),
	)

	ctx := testUserContext()
	resp, err := handler.GetKnowledgeHome(ctx, connect.NewRequest(&knowledgehomev1.GetKnowledgeHomeRequest{Limit: 20}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Items, 1)
	require.NotNil(t, resp.Msg.ServiceQuality)
	assert.Equal(t, "full", *resp.Msg.ServiceQuality)
}

func TestHandler_GetKnowledgeHome_ServiceQualityFallback(t *testing.T) {
	logger.InitLogger()
	handler, homePort, _ := setupHandler()
	homePort.items = nil
	homePort.err = errors.New("items unavailable")

	ctx := testUserContext()
	req := connect.NewRequest(&knowledgehomev1.GetKnowledgeHomeRequest{Limit: 20})

	resp, err := handler.GetKnowledgeHome(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.ServiceQuality)
	assert.Equal(t, "fallback", *resp.Msg.ServiceQuality)
}

func TestHandler_GetKnowledgeHome_BothReadModelsFail_ReturnsInternalError(t *testing.T) {
	logger.InitLogger()
	handler, homePort, digestPort := setupHandler()
	homePort.items = nil
	homePort.err = errors.New("items unavailable")
	digestPort.err = errors.New("digest unavailable")

	ctx := testUserContext()
	req := connect.NewRequest(&knowledgehomev1.GetKnowledgeHomeRequest{Limit: 20})

	_, err := handler.GetKnowledgeHome(ctx, req)
	require.Error(t, err)

	connectErr, ok := err.(*connect.Error)
	require.True(t, ok)
	assert.Equal(t, connect.CodeInternal, connectErr.Code())
}
