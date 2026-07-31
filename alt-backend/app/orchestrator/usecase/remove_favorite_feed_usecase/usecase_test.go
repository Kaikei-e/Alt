package remove_favorite_feed_usecase

import (
	"alt/domain"
	"alt/mocks"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

// authedContext builds the request context the middleware installs. The
// usecase resolves the reader from it now: past this point the call leaves the
// process that knows who is logged in (capability catalog §2.I).
func authedContext(userID uuid.UUID) context.Context {
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

func TestRemoveFavoriteFeedUsecase_Execute(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		userID := uuid.New()
		mockGateway := mocks.NewMockRegisterFavoriteFeedPort(ctrl)
		mockGateway.EXPECT().RemoveFavoriteFeed(gomock.Any(), "https://example.com/feed", userID).Return(nil)

		uc := NewRemoveFavoriteFeedUsecase(mockGateway)
		err := uc.Execute(authedContext(userID), "https://example.com/feed")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("no authenticated user", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockGateway := mocks.NewMockRegisterFavoriteFeedPort(ctrl)
		uc := NewRemoveFavoriteFeedUsecase(mockGateway)
		if err := uc.Execute(context.Background(), "https://example.com/feed"); err == nil {
			t.Fatal("expected an authentication error")
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

		userID := uuid.New()
		mockGateway := mocks.NewMockRegisterFavoriteFeedPort(ctrl)
		mockGateway.EXPECT().RemoveFavoriteFeed(gomock.Any(), "https://example.com/feed", userID).Return(errors.New("db error"))

		uc := NewRemoveFavoriteFeedUsecase(mockGateway)
		err := uc.Execute(authedContext(userID), "https://example.com/feed")
		if err == nil || err.Error() != "db error" {
			t.Fatalf("expected gateway error, got %v", err)
		}
	})
}
