package register_favorite_feed_usecase

import (
	"alt/mocks"
	"context"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestRegisterFavoriteFeedUsecase_Execute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGateway := mocks.NewMockRegisterFavoriteFeedPort(ctrl)

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
				mockGateway.EXPECT().RegisterFavoriteFeed(gomock.Any(), "https://example.com/rss.xml").Return(nil)
			},
			wantErr: false,
		},
		{
			name: "gateway error",
			url:  "https://example.com/rss.xml",
			setupMock: func() {
				mockGateway.EXPECT().RegisterFavoriteFeed(gomock.Any(), "https://example.com/rss.xml").Return(context.DeadlineExceeded)
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			u := NewRegisterFavoriteFeedUsecase(mockGateway)
			err := u.Execute(context.Background(), tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
