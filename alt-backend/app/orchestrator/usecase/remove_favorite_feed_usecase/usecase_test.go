package remove_favorite_feed_usecase

import (
	"alt/mocks"
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestRemoveFavoriteFeedUsecase_Execute(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockGateway := mocks.NewMockRegisterFavoriteFeedPort(ctrl)
		mockGateway.EXPECT().RemoveFavoriteFeed(gomock.Any(), "https://example.com/feed").Return(nil)

		uc := NewRemoveFavoriteFeedUsecase(mockGateway)
		err := uc.Execute(context.Background(), "https://example.com/feed")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("empty URL returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockGateway := mocks.NewMockRegisterFavoriteFeedPort(ctrl)
		uc := NewRemoveFavoriteFeedUsecase(mockGateway)
		err := uc.Execute(context.Background(), "  ")
		if err == nil || err.Error() != "feed url cannot be empty" {
			t.Fatalf("expected empty URL error, got %v", err)
		}
	})

	t.Run("gateway error is propagated", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockGateway := mocks.NewMockRegisterFavoriteFeedPort(ctrl)
		mockGateway.EXPECT().RemoveFavoriteFeed(gomock.Any(), "https://example.com/feed").Return(errors.New("db error"))

		uc := NewRemoveFavoriteFeedUsecase(mockGateway)
		err := uc.Execute(context.Background(), "https://example.com/feed")
		if err == nil || err.Error() != "db error" {
			t.Fatalf("expected gateway error, got %v", err)
		}
	})
}
