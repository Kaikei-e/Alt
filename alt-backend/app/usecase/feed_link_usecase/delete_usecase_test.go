package feed_link_usecase

import (
	"context"
	"errors"
	"testing"

	"alt/mocks"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestDeleteFeedLinkUsecase_Execute_UnsubscribesUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	port := mocks.NewMockSubscriptionPort(ctrl)
	usecase := NewDeleteFeedLinkUsecase(port)
	userID := uuid.New()
	feedLinkID := uuid.New()

	port.EXPECT().Unsubscribe(gomock.Any(), userID, feedLinkID).Return(nil)

	if err := usecase.Execute(context.Background(), userID, feedLinkID); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestDeleteFeedLinkUsecase_Execute_PropagatesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	port := mocks.NewMockSubscriptionPort(ctrl)
	usecase := NewDeleteFeedLinkUsecase(port)
	userID := uuid.New()
	feedLinkID := uuid.New()
	wantErr := errors.New("unsubscribe failed")

	port.EXPECT().Unsubscribe(gomock.Any(), userID, feedLinkID).Return(wantErr)

	if err := usecase.Execute(context.Background(), userID, feedLinkID); !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}
