package search_feed_usecase

import (
	"alt/domain"
	"alt/mocks"
	"context"
	"errors"
	"fmt"
	"testing"

	"go.uber.org/mock/gomock"
)

func generateMockData(amount int) []*domain.FeedItem {
	items := make([]*domain.FeedItem, amount)
	for i := 0; i < amount; i++ {
		items[i] = &domain.FeedItem{
			Title:       fmt.Sprintf("test %d", i),
			Link:        fmt.Sprintf("https://example.com/feed/%d", i),
			Description: fmt.Sprintf("test %d", i),
			Published:   "2021-01-01",
		}
	}
	return items
}

func TestSearchFeedTitleUsecase_Execute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	type args struct {
		ctx   context.Context
		query string
	}

	t.Run("should return 10 results", func(t *testing.T) {
		mockSearchByTitleGateway := mocks.NewMockSearchByTitlePort(ctrl)
		mockSearchByTitleGateway.EXPECT().SearchByTitle(ctx, "test").Return(generateMockData(10), nil)

		usecase := NewSearchFeedTitleUsecase(mockSearchByTitleGateway)
		results, err := usecase.Execute(ctx, "test")
		if err != nil {
			t.Fatalf("Failed to execute usecase: %v", err)
		}
		if len(results) != 10 {
			t.Fatalf("Expected 10 results, got %d", len(results))
		}

	})

	t.Run("should return 0 results", func(t *testing.T) {
		mockSearchByTitleGateway := mocks.NewMockSearchByTitlePort(ctrl)
		mockSearchByTitleGateway.EXPECT().SearchByTitle(ctx, "").Return([]*domain.FeedItem{}, nil)

		usecase := NewSearchFeedTitleUsecase(mockSearchByTitleGateway)
		results, err := usecase.Execute(ctx, "")
		if err != nil {
			t.Fatalf("Failed to execute usecase: %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("Expected 0 results, got %d", len(results))
		}
	})

	t.Run("should return error", func(t *testing.T) {
		mockSearchByTitleGateway := mocks.NewMockSearchByTitlePort(ctrl)
		mockSearchByTitleGateway.EXPECT().SearchByTitle(ctx, "test").Return(nil, errors.New("error"))

		usecase := NewSearchFeedTitleUsecase(mockSearchByTitleGateway)
		results, err := usecase.Execute(ctx, "test")
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if len(results) != 0 {
			t.Fatalf("Expected 0 results, got %d", len(results))
		}
	})
}
