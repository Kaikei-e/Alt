package recap_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"alt/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockRecapPort is a mock implementation of RecapPort
type MockRecapPort struct {
	mock.Mock
}

func (m *MockRecapPort) GetSevenDayRecap(ctx context.Context) (*domain.RecapSummary, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.RecapSummary), args.Error(1)
}

func (m *MockRecapPort) GetEveningPulse(ctx context.Context, date string) (*domain.EveningPulse, error) {
	args := m.Called(ctx, date)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.EveningPulse), args.Error(1)
}

func TestRecapUsecase_GetEveningPulse(t *testing.T) {
	t.Run("success - delegates to port and returns result", func(t *testing.T) {
		mockPort := new(MockRecapPort)
		expectedPulse := &domain.EveningPulse{
			JobID:       "test-job-123",
			Date:        "2026-01-31",
			GeneratedAt: time.Now(),
			Status:      domain.PulseStatusNormal,
			Topics: []domain.PulseTopic{
				{
					ClusterID: 12345,
					Role:      domain.TopicRoleNeedToKnow,
					Title:     "Test Topic",
				},
			},
		}

		mockPort.On("GetEveningPulse", mock.Anything, "2026-01-31").Return(expectedPulse, nil)

		uc := NewRecapUsecase(mockPort)
		result, err := uc.GetEveningPulse(context.Background(), "2026-01-31")

		require.NoError(t, err)
		assert.Equal(t, expectedPulse, result)
		mockPort.AssertExpectations(t)
	})

	t.Run("success - empty date delegates correctly", func(t *testing.T) {
		mockPort := new(MockRecapPort)
		expectedPulse := &domain.EveningPulse{
			JobID:  "test-job-456",
			Date:   "2026-01-31",
			Status: domain.PulseStatusNormal,
		}

		mockPort.On("GetEveningPulse", mock.Anything, "").Return(expectedPulse, nil)

		uc := NewRecapUsecase(mockPort)
		result, err := uc.GetEveningPulse(context.Background(), "")

		require.NoError(t, err)
		assert.Equal(t, expectedPulse, result)
		mockPort.AssertExpectations(t)
	})

	t.Run("not found - propagates error", func(t *testing.T) {
		mockPort := new(MockRecapPort)
		mockPort.On("GetEveningPulse", mock.Anything, "2026-01-31").
			Return(nil, domain.ErrEveningPulseNotFound)

		uc := NewRecapUsecase(mockPort)
		_, err := uc.GetEveningPulse(context.Background(), "2026-01-31")

		assert.ErrorIs(t, err, domain.ErrEveningPulseNotFound)
		mockPort.AssertExpectations(t)
	})

	t.Run("error - propagates generic error", func(t *testing.T) {
		mockPort := new(MockRecapPort)
		expectedErr := errors.New("network error")
		mockPort.On("GetEveningPulse", mock.Anything, "2026-01-31").
			Return(nil, expectedErr)

		uc := NewRecapUsecase(mockPort)
		_, err := uc.GetEveningPulse(context.Background(), "2026-01-31")

		assert.ErrorIs(t, err, expectedErr)
		mockPort.AssertExpectations(t)
	})

	t.Run("quiet day - returns quiet day result", func(t *testing.T) {
		mockPort := new(MockRecapPort)
		expectedPulse := &domain.EveningPulse{
			JobID:  "quiet-job",
			Date:   "2026-01-31",
			Status: domain.PulseStatusQuietDay,
			Topics: []domain.PulseTopic{},
			QuietDay: &domain.QuietDayInfo{
				Message: "今日は静かな一日でした。",
				WeeklyHighlights: []domain.WeeklyHighlight{
					{ID: "h1", Title: "Weekly top news", Date: "2026-01-29", Role: "need_to_know"},
				},
			},
		}

		mockPort.On("GetEveningPulse", mock.Anything, "2026-01-31").Return(expectedPulse, nil)

		uc := NewRecapUsecase(mockPort)
		result, err := uc.GetEveningPulse(context.Background(), "2026-01-31")

		require.NoError(t, err)
		assert.Equal(t, domain.PulseStatusQuietDay, result.Status)
		require.NotNil(t, result.QuietDay)
		assert.Len(t, result.QuietDay.WeeklyHighlights, 1)
		mockPort.AssertExpectations(t)
	})
}
