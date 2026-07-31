package register_favorite_feed_usecase

import (
	"alt/domain"
	"alt/mocks"
	"context"
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

func TestRegisterFavoriteFeedUsecase_Execute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGateway := mocks.NewMockRegisterFavoriteFeedPort(ctrl)
	userID := uuid.New()

	tests := []struct {
		name      string
		url       string
		setupMock func()
		wantErr   bool
	}{
		{
			name: "success",
			url:  "https://example.com/rss.xml",
			setupMock: func() {
				mockGateway.EXPECT().RegisterFavoriteFeed(gomock.Any(), "https://example.com/rss.xml", userID).Return(nil)
			},
			wantErr: false,
		},
		{
			name: "gateway error",
			url:  "https://example.com/rss.xml",
			setupMock: func() {
				mockGateway.EXPECT().RegisterFavoriteFeed(gomock.Any(), "https://example.com/rss.xml", userID).Return(context.DeadlineExceeded)
			},
			wantErr: true,
		},
		{
			name:      "empty url",
			url:       "",
			setupMock: func() {},
			wantErr:   true,
		},
	}

	// The unauthenticated case is separate because it must not reach the
	// gateway at all: with the tenant carried explicitly across the boundary,
	// a call with nobody logged in would otherwise send the zero UUID and star
	// a feed for a user that does not exist.
	t.Run("no authenticated user", func(t *testing.T) {
		u := NewRegisterFavoriteFeedUsecase(mockGateway)
		if err := u.Execute(context.Background(), "https://example.com/rss.xml"); err == nil {
			t.Error("expected an authentication error")
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			u := NewRegisterFavoriteFeedUsecase(mockGateway)
			err := u.Execute(authedContext(userID), tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
