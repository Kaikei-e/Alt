package search_feed_usecase

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

func TestSearchFeedByTitleUsecase_Execute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userID := uuid.New()
	userCtx := &domain.UserContext{
		UserID:    userID,
		Email:     "user@example.com",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	validCtx := domain.SetUserContext(context.Background(), userCtx)

	mockFeeds := []*domain.FeedItem{
		{Title: "Go News", Description: "Go latest", Link: "https://golang.org"},
	}

	tests := []struct {
		name        string
		ctx         context.Context
		query       string
		setupMock   func(m *mocks.MockSearchByTitlePort)
		expectError bool
		wantCount   int
	}{
		{
			name:  "fails when user context is missing",
			ctx:   context.Background(),
			query: "test",
			setupMock: func(m *mocks.MockSearchByTitlePort) {
				// No call expected when user context is missing.
			},
			expectError: true,
		},
		{
			name:  "succeeds when port returns feeds",
			ctx:   validCtx,
			query: "Go",
			setupMock: func(m *mocks.MockSearchByTitlePort) {
				m.EXPECT().SearchFeedsByTitle(validCtx, "Go", userID.String()).Return(mockFeeds, nil)
			},
			expectError: false,
			wantCount:   1,
		},
		{
			name:  "returns error when port fails",
			ctx:   validCtx,
			query: "Go",
			setupMock: func(m *mocks.MockSearchByTitlePort) {
				m.EXPECT().SearchFeedsByTitle(validCtx, "Go", userID.String()).Return(nil, errors.New("db error"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPort := mocks.NewMockSearchByTitlePort(ctrl)
			tt.setupMock(mockPort)

			uc := NewSearchFeedByTitleUsecase(mockPort)
			results, err := uc.Execute(tt.ctx, tt.query)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(results) != tt.wantCount {
					t.Fatalf("expected %d results, got %d", tt.wantCount, len(results))
				}
			}
		})
	}
}
