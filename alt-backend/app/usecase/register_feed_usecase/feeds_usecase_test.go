package register_feed_usecase

import (
	"alt/mocks"
	"alt/usecase/testutil"
	"alt/utils/logger"
	"context"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestRegisterFeedUsecase_Execute(t *testing.T) {
	// Initialize logger to prevent nil pointer dereference
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetchFeedGateway := mocks.NewMockFetchFeedsPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	tests := []struct {
		name      string
		ctx       context.Context
		link      string
		mockSetup func()
		wantErr   bool
	}{
		{
			name: "successful registration with feeds",
			ctx:  context.Background(),
			link: "https://example.com/rss/news",
			mockSetup: func() {
				mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
				mockFetchFeedGateway.EXPECT().FetchFeeds(gomock.Any(), "https://example.com/rss/news").Return(mockData, nil).Times(1)
				mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			wantErr: false,
		},
		{
			name: "successful registration with empty feeds",
			ctx:  context.Background(),
			link: "https://example.com/rss/empty",
			mockSetup: func() {
				emptyFeeds := testutil.CreateEmptyFeedItems()
				mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "https://example.com/rss/empty").Return(nil).Times(1)
				mockFetchFeedGateway.EXPECT().FetchFeeds(gomock.Any(), "https://example.com/rss/empty").Return(emptyFeeds, nil).Times(1)
				mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			wantErr: false,
		},
		{
			name: "error registering RSS feed link",
			ctx:  context.Background(),
			link: "https://example.com/rss/error",
			mockSetup: func() {
				mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "https://example.com/rss/error").Return(testutil.ErrMockDatabase).Times(1)
				// Should not call other methods if first step fails
			},
			wantErr: true,
		},
		{
			name: "error fetching feeds",
			ctx:  context.Background(),
			link: "https://example.com/rss/fetch-error",
			mockSetup: func() {
				mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "https://example.com/rss/fetch-error").Return(nil).Times(1)
				mockFetchFeedGateway.EXPECT().FetchFeeds(gomock.Any(), "https://example.com/rss/fetch-error").Return(nil, testutil.ErrMockNetwork).Times(1)
				// Should not call register feeds if fetch fails
			},
			wantErr: true,
		},
		{
			name: "error registering feeds",
			ctx:  context.Background(),
			link: "https://example.com/rss/register-error",
			mockSetup: func() {
				mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "https://example.com/rss/register-error").Return(nil).Times(1)
				mockFetchFeedGateway.EXPECT().FetchFeeds(gomock.Any(), "https://example.com/rss/register-error").Return(mockData, nil).Times(1)
				mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return(testutil.ErrMockDatabase).Times(1)
			},
			wantErr: true,
		},
		{
			name: "context cancellation",
			ctx:  testutil.CreateCancelledContext(),
			link: "https://example.com/rss/cancelled",
			mockSetup: func() {
				mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "https://example.com/rss/cancelled").Return(context.Canceled).Times(1)
			},
			wantErr: true,
		},
		{
			name: "invalid URL format",
			ctx:  context.Background(),
			link: "not-a-valid-url",
			mockSetup: func() {
				mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "not-a-valid-url").Return(testutil.ErrMockValidation).Times(1)
			},
			wantErr: true,
		},
		{
			name: "empty URL",
			ctx:  context.Background(),
			link: "",
			mockSetup: func() {
				mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "").Return(testutil.ErrMockValidation).Times(1)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()
			r := NewRegisterFeedsUsecase(mockRegisterFeedLinkPort, mockRegisterFeedsPort, mockFetchFeedGateway)
			err := r.Execute(tt.ctx, tt.link)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterFeedUsecase.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
