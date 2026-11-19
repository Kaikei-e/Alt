package recap_usecase

import (
	"context"
	"errors"
	"testing"

	"alt/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockRecapPort is a mock implementation of RecapPort
type MockRecapPort struct {
	mock.Mock
}

func (m *MockRecapPort) GetThreeDayRecap(ctx context.Context) (*domain.RecapSummary, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.RecapSummary), args.Error(1)
}

func (m *MockRecapPort) GetTopicCards(ctx context.Context) (*domain.RecapCardsResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.RecapCardsResponse), args.Error(1)
}

type MockRecapSearchPort struct {
	mock.Mock
}

func (m *MockRecapSearchPort) SearchRecapsByTag(ctx context.Context, tagName string, limit int) ([]*domain.RecapSearchResult, error) {
	args := m.Called(ctx, tagName, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.RecapSearchResult), args.Error(1)
}

func (m *MockRecapSearchPort) SearchRecapsByQuery(ctx context.Context, query string, limit int) ([]*domain.RecapSearchResult, int64, error) {
	args := m.Called(ctx, query, limit)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*domain.RecapSearchResult), args.Get(1).(int64), args.Error(2)
}

func TestRecapUsecase_GetTopicCards(t *testing.T) {
	t.Run("success - returns cards response with completed surface_status", func(t *testing.T) {
		mockPort := new(MockRecapPort)
		expected := &domain.RecapCardsResponse{
			Job: &domain.RecapCardsJob{
				JobID:         "job-1",
				KickedAt:      "2026-09-22T17:00:00Z",
				From:          "2026-09-19T17:00:00Z",
				To:            "2026-09-22T17:00:00Z",
				ParamsVersion: "cards-v0.2",
				CardsSelected: 1,
				Degraded:      false,
			},
			Cards: []*domain.RecapCard{
				{
					ID:         "card-1",
					Rank:       1,
					StoryID:    "story-1",
					HeadlineJa: "見出し",
					WhatJa:     "内容[1]",
					CreatedAt:  "2026-09-22T17:05:00Z",
				},
			},
			SurfaceStatus: &domain.RecapSurfaceStatus{
				GeneratedAt: "2026-09-22T17:05:00Z",
				LatestRun: &domain.RecapSurfaceRun{
					JobID:     "job-1",
					Status:    "completed",
					KickedAt:  "2026-09-22T17:00:00Z",
					UpdatedAt: "2026-09-22T17:05:00Z",
				},
			},
		}

		mockPort.On("GetTopicCards", mock.Anything).Return(expected, nil)

		uc := NewRecapUsecase(mockPort, nil)
		result, err := uc.GetTopicCards(context.Background())

		require.NoError(t, err)
		assert.Equal(t, expected, result)
		mockPort.AssertExpectations(t)
	})

	t.Run("success - empty cards response with nil surface_status", func(t *testing.T) {
		mockPort := new(MockRecapPort)
		expected := &domain.RecapCardsResponse{
			Job:           nil,
			Cards:         []*domain.RecapCard{},
			SurfaceStatus: nil,
		}

		mockPort.On("GetTopicCards", mock.Anything).Return(expected, nil)

		uc := NewRecapUsecase(mockPort, nil)
		result, err := uc.GetTopicCards(context.Background())

		require.NoError(t, err)
		assert.Nil(t, result.Job)
		assert.Empty(t, result.Cards)
		assert.Nil(t, result.SurfaceStatus)
		mockPort.AssertExpectations(t)
	})

	t.Run("error - port failure propagates", func(t *testing.T) {
		mockPort := new(MockRecapPort)
		mockPort.On("GetTopicCards", mock.Anything).Return(nil, errors.New("upstream failure"))

		uc := NewRecapUsecase(mockPort, nil)
		result, err := uc.GetTopicCards(context.Background())

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, "upstream failure", err.Error())
		mockPort.AssertExpectations(t)
	})
}

func TestRecapUsecase_SearchRecapsByTag(t *testing.T) {
	t.Run("success - delegates to recapSearch", func(t *testing.T) {
		mockSearch := new(MockRecapSearchPort)
		expected := []*domain.RecapSearchResult{
			{Genre: "Technology", Summary: "Tech news summary"},
		}
		mockSearch.On("SearchRecapsByTag", mock.Anything, "tech", 10).Return(expected, nil)

		uc := NewRecapUsecase(nil, mockSearch)
		result, err := uc.SearchRecapsByTag(context.Background(), "tech", 10)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
		mockSearch.AssertExpectations(t)
	})
}

func TestRecapUsecase_SearchRecapsByQuery(t *testing.T) {
	t.Run("success - delegates to recapSearch", func(t *testing.T) {
		mockSearch := new(MockRecapSearchPort)
		expected := []*domain.RecapSearchResult{
			{Genre: "AI", Summary: "AI summary"},
		}
		mockSearch.On("SearchRecapsByQuery", mock.Anything, "ai", 5).Return(expected, int64(1), nil)

		uc := NewRecapUsecase(nil, mockSearch)
		result, err := uc.SearchRecapsByQuery(context.Background(), "ai", 5)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
		mockSearch.AssertExpectations(t)
	})
}
