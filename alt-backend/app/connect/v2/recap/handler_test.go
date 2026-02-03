package recap

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"

	recapv2 "alt/gen/proto/alt/recap/v2"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockRecapUsecase is a mock implementation of RecapUsecase
type MockRecapUsecase struct {
	mock.Mock
}

func (m *MockRecapUsecase) GetSevenDayRecap(ctx context.Context) (*domain.RecapSummary, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.RecapSummary), args.Error(1)
}

func (m *MockRecapUsecase) GetThreeDayRecap(ctx context.Context) (*domain.RecapSummary, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.RecapSummary), args.Error(1)
}

func (m *MockRecapUsecase) GetEveningPulse(ctx context.Context, date string) (*domain.EveningPulse, error) {
	args := m.Called(ctx, date)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.EveningPulse), args.Error(1)
}

func TestHandler_GetEveningPulse(t *testing.T) {
	logger := slog.Default()

	t.Run("success - authenticated user with 3 topics", func(t *testing.T) {
		mockUsecase := new(MockRecapUsecase)
		tier1Count := 5
		trendMultiplier := 4.2
		genre := "Technology"
		expectedPulse := &domain.EveningPulse{
			JobID:       "job-123",
			Date:        "2026-01-31",
			GeneratedAt: time.Date(2026, 1, 31, 18, 0, 0, 0, time.UTC),
			Status:      domain.PulseStatusNormal,
			Topics: []domain.PulseTopic{
				{
					ClusterID:    12345,
					Role:         domain.TopicRoleNeedToKnow,
					Title:        "日銀利上げ決定",
					Rationale:    domain.PulseRationale{Text: "12媒体が報道", Confidence: domain.ConfidenceHigh},
					ArticleCount: 45,
					SourceCount:  12,
					Tier1Count:   &tier1Count,
					TimeAgo:      "3時間前",
					Genre:        &genre,
					ArticleIDs:   []string{"art-001", "art-002"},
				},
				{
					ClusterID:       12346,
					Role:            domain.TopicRoleTrend,
					Title:           "AI半導体急騰",
					Rationale:       domain.PulseRationale{Text: "4.2倍", Confidence: domain.ConfidenceHigh},
					ArticleCount:    28,
					SourceCount:     8,
					TimeAgo:         "1時間前",
					TrendMultiplier: &trendMultiplier,
					ArticleIDs:      []string{"art-010"},
				},
				{
					ClusterID:    12347,
					Role:         domain.TopicRoleSerendipity,
					Title:        "深海新種発見",
					Rationale:    domain.PulseRationale{Text: "Science", Confidence: domain.ConfidenceMedium},
					ArticleCount: 5,
					SourceCount:  3,
					TimeAgo:      "5時間前",
					ArticleIDs:   []string{"art-020"},
				},
			},
		}

		mockUsecase.On("GetEveningPulse", mock.Anything, "2026-01-31").Return(expectedPulse, nil)

		handler := NewHandlerWithUsecase(mockUsecase, nil, logger)
		ctx := domain.SetUserContext(context.Background(), &domain.UserContext{UserID: uuid.New(), Email: "test@example.com", ExpiresAt: time.Now().Add(time.Hour)})

		date := "2026-01-31"
		req := connect.NewRequest(&recapv2.GetEveningPulseRequest{Date: &date})
		resp, err := handler.GetEveningPulse(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "job-123", resp.Msg.JobId)
		assert.Equal(t, "2026-01-31", resp.Msg.Date)
		assert.Equal(t, recapv2.PulseStatus_PULSE_STATUS_NORMAL, resp.Msg.Status)
		assert.Len(t, resp.Msg.Topics, 3)

		// Verify first topic
		assert.Equal(t, int64(12345), resp.Msg.Topics[0].ClusterId)
		assert.Equal(t, recapv2.TopicRole_TOPIC_ROLE_NEED_TO_KNOW, resp.Msg.Topics[0].Role)
		assert.Equal(t, "日銀利上げ決定", resp.Msg.Topics[0].Title)
		require.NotNil(t, resp.Msg.Topics[0].Tier1Count)
		assert.Equal(t, int32(5), *resp.Msg.Topics[0].Tier1Count)

		// Verify second topic (Trend)
		assert.Equal(t, recapv2.TopicRole_TOPIC_ROLE_TREND, resp.Msg.Topics[1].Role)
		require.NotNil(t, resp.Msg.Topics[1].TrendMultiplier)
		assert.InDelta(t, 4.2, *resp.Msg.Topics[1].TrendMultiplier, 0.01)

		mockUsecase.AssertExpectations(t)
	})

	t.Run("success - quiet day", func(t *testing.T) {
		mockUsecase := new(MockRecapUsecase)
		expectedPulse := &domain.EveningPulse{
			JobID:       "quiet-job",
			Date:        "2026-01-31",
			GeneratedAt: time.Now(),
			Status:      domain.PulseStatusQuietDay,
			Topics:      []domain.PulseTopic{},
			QuietDay: &domain.QuietDayInfo{
				Message: "今日は静かな一日でした。",
				WeeklyHighlights: []domain.WeeklyHighlight{
					{ID: "h1", Title: "Top News", Date: "2026-01-29", Role: "need_to_know"},
				},
			},
		}

		mockUsecase.On("GetEveningPulse", mock.Anything, "").Return(expectedPulse, nil)

		handler := NewHandlerWithUsecase(mockUsecase, nil, logger)
		ctx := domain.SetUserContext(context.Background(), &domain.UserContext{UserID: uuid.New(), Email: "test@example.com", ExpiresAt: time.Now().Add(time.Hour)})

		req := connect.NewRequest(&recapv2.GetEveningPulseRequest{})
		resp, err := handler.GetEveningPulse(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, recapv2.PulseStatus_PULSE_STATUS_QUIET_DAY, resp.Msg.Status)
		assert.Len(t, resp.Msg.Topics, 0)
		require.NotNil(t, resp.Msg.QuietDay)
		assert.Contains(t, resp.Msg.QuietDay.Message, "静かな一日")
		assert.Len(t, resp.Msg.QuietDay.WeeklyHighlights, 1)

		mockUsecase.AssertExpectations(t)
	})

	t.Run("unauthenticated - returns error", func(t *testing.T) {
		mockUsecase := new(MockRecapUsecase)
		handler := NewHandlerWithUsecase(mockUsecase, nil, logger)

		// No user context
		req := connect.NewRequest(&recapv2.GetEveningPulseRequest{})
		_, err := handler.GetEveningPulse(context.Background(), req)

		require.Error(t, err)
		connectErr, ok := err.(*connect.Error)
		require.True(t, ok)
		assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())

		mockUsecase.AssertNotCalled(t, "GetEveningPulse")
	})

	t.Run("not found - returns not found error", func(t *testing.T) {
		mockUsecase := new(MockRecapUsecase)
		mockUsecase.On("GetEveningPulse", mock.Anything, "2026-01-31").
			Return(nil, domain.ErrEveningPulseNotFound)

		handler := NewHandlerWithUsecase(mockUsecase, nil, logger)
		ctx := domain.SetUserContext(context.Background(), &domain.UserContext{UserID: uuid.New(), Email: "test@example.com", ExpiresAt: time.Now().Add(time.Hour)})

		date := "2026-01-31"
		req := connect.NewRequest(&recapv2.GetEveningPulseRequest{Date: &date})
		_, err := handler.GetEveningPulse(ctx, req)

		require.Error(t, err)
		connectErr, ok := err.(*connect.Error)
		require.True(t, ok)
		assert.Equal(t, connect.CodeNotFound, connectErr.Code())

		mockUsecase.AssertExpectations(t)
	})

	t.Run("internal error - returns internal error", func(t *testing.T) {
		mockUsecase := new(MockRecapUsecase)
		mockUsecase.On("GetEveningPulse", mock.Anything, "").
			Return(nil, errors.New("database error"))

		handler := NewHandlerWithUsecase(mockUsecase, nil, logger)
		ctx := domain.SetUserContext(context.Background(), &domain.UserContext{UserID: uuid.New(), Email: "test@example.com", ExpiresAt: time.Now().Add(time.Hour)})

		req := connect.NewRequest(&recapv2.GetEveningPulseRequest{})
		_, err := handler.GetEveningPulse(ctx, req)

		require.Error(t, err)
		connectErr, ok := err.(*connect.Error)
		require.True(t, ok)
		assert.Equal(t, connect.CodeInternal, connectErr.Code())

		mockUsecase.AssertExpectations(t)
	})
}
