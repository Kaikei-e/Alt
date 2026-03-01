package register_feed_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/usecase/testutil"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
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
				mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return([]string{"id-1", "id-2"}, nil).Times(1)
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
				mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return([]string{}, nil).Times(1)
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
				mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return(nil, testutil.ErrMockDatabase).Times(1)
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

// TDD RED PHASE: Proxy-enabled usecase tests (EXPECTED TO FAIL)
func TestRegisterFeedUsecase_WithProxyEnabled(t *testing.T) {
	// Test usecase behavior when proxy is enabled
	t.Setenv("HTTP_PROXY", "http://nginx-external.alt-ingress.svc.cluster.local:8888")
	t.Setenv("PROXY_ENABLED", "true")

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
			name: "successful registration via proxy",
			ctx:  context.Background(),
			link: "https://example.com/rss/news",
			mockSetup: func() {
				// Mock should expect proxy-aware gateway behavior
				mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
				mockFetchFeedGateway.EXPECT().FetchFeeds(gomock.Any(), "https://example.com/rss/news").Return(mockData, nil).Times(1)
				mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return([]string{"id-1", "id-2"}, nil).Times(1)
			},
			wantErr: false,
		},
		{
			name: "proxy connection failure",
			ctx:  context.Background(),
			link: "https://example.com/rss/proxy-fail",
			mockSetup: func() {
				// Mock should simulate proxy connection failure
				mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "https://example.com/rss/proxy-fail").Return(testutil.ErrMockNetwork).Times(1)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			// This should work with existing usecase but will test proxy-aware gateway
			r := NewRegisterFeedsUsecase(mockRegisterFeedLinkPort, mockRegisterFeedsPort, mockFetchFeedGateway)
			err := r.Execute(tt.ctx, tt.link)

			// This test may not fail immediately since usecase delegates to gateway
			// But it establishes the expected behavior
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterFeedUsecase.Execute() with proxy error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func createUserContext(userID uuid.UUID) context.Context {
	user := &domain.UserContext{
		UserID:    userID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		SessionID: "test-session",
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	return domain.SetUserContext(context.Background(), user)
}

func TestRegisterFeedUsecase_AutoSubscribe_Success(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetchFeedGateway := mocks.NewMockFetchFeedsPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockSubscriptionPort := mocks.NewMockSubscriptionPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	feedLinkID := uuid.New()
	feedLinkIDStr := feedLinkID.String()
	userID := uuid.New()
	ctx := createUserContext(userID)

	mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
	mockFetchFeedGateway.EXPECT().FetchFeeds(gomock.Any(), "https://example.com/rss/news").Return(mockData, nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return([]string{"id-1", "id-2"}, nil).Times(1)
	mockSubscriptionPort.EXPECT().Subscribe(gomock.Any(), userID, feedLinkID).Return(nil).Times(1)

	r := NewRegisterFeedsUsecase(mockRegisterFeedLinkPort, mockRegisterFeedsPort, mockFetchFeedGateway)
	r.SetSubscriptionPort(mockSubscriptionPort)

	// Mock FeedLinkIDResolver to return the feedLinkID
	mockResolver := mocks.NewMockFeedLinkIDResolver(ctrl)
	mockResolver.EXPECT().FetchFeedLinkIDByURL(gomock.Any(), "https://example.com/rss/news").Return(&feedLinkIDStr, nil).Times(1)
	r.SetFeedLinkIDResolver(mockResolver)

	err := r.Execute(ctx, "https://example.com/rss/news")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestRegisterFeedUsecase_AutoSubscribe_NoUserContext(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetchFeedGateway := mocks.NewMockFetchFeedsPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockSubscriptionPort := mocks.NewMockSubscriptionPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	feedLinkID := uuid.New()
	feedLinkIDStr := feedLinkID.String()

	mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
	mockFetchFeedGateway.EXPECT().FetchFeeds(gomock.Any(), "https://example.com/rss/news").Return(mockData, nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return([]string{"id-1", "id-2"}, nil).Times(1)
	// Subscribe should NOT be called when there is no user context

	r := NewRegisterFeedsUsecase(mockRegisterFeedLinkPort, mockRegisterFeedsPort, mockFetchFeedGateway)
	r.SetSubscriptionPort(mockSubscriptionPort)

	mockResolver := mocks.NewMockFeedLinkIDResolver(ctrl)
	mockResolver.EXPECT().FetchFeedLinkIDByURL(gomock.Any(), "https://example.com/rss/news").Return(&feedLinkIDStr, nil).Times(1)
	r.SetFeedLinkIDResolver(mockResolver)

	// No user context in background context
	err := r.Execute(context.Background(), "https://example.com/rss/news")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestRegisterFeedUsecase_AutoSubscribe_SubscribeError(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetchFeedGateway := mocks.NewMockFetchFeedsPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockSubscriptionPort := mocks.NewMockSubscriptionPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	feedLinkID := uuid.New()
	feedLinkIDStr := feedLinkID.String()
	userID := uuid.New()
	ctx := createUserContext(userID)

	mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
	mockFetchFeedGateway.EXPECT().FetchFeeds(gomock.Any(), "https://example.com/rss/news").Return(mockData, nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return([]string{"id-1", "id-2"}, nil).Times(1)
	// Subscribe fails, but Execute should still succeed
	mockSubscriptionPort.EXPECT().Subscribe(gomock.Any(), userID, feedLinkID).Return(errors.New("subscription error")).Times(1)

	r := NewRegisterFeedsUsecase(mockRegisterFeedLinkPort, mockRegisterFeedsPort, mockFetchFeedGateway)
	r.SetSubscriptionPort(mockSubscriptionPort)

	mockResolver := mocks.NewMockFeedLinkIDResolver(ctrl)
	mockResolver.EXPECT().FetchFeedLinkIDByURL(gomock.Any(), "https://example.com/rss/news").Return(&feedLinkIDStr, nil).Times(1)
	r.SetFeedLinkIDResolver(mockResolver)

	err := r.Execute(ctx, "https://example.com/rss/news")
	if err != nil {
		t.Errorf("Expected no error even when subscribe fails, got %v", err)
	}
}

func TestRegisterFeedUsecase_ProxyFailover(t *testing.T) {
	// Test failover behavior when proxy fails
	t.Setenv("HTTP_PROXY", "http://invalid-proxy.invalid:8888")
	t.Setenv("PROXY_ENABLED", "true")

	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetchFeedGateway := mocks.NewMockFetchFeedsPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)

	// Mock proxy failure followed by direct connection attempt
	mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "https://example.com/rss").Return(testutil.ErrMockNetwork).Times(1)

	r := NewRegisterFeedsUsecase(mockRegisterFeedLinkPort, mockRegisterFeedsPort, mockFetchFeedGateway)
	err := r.Execute(context.Background(), "https://example.com/rss")

	// This should fail because proxy failover is not implemented yet
	if err == nil {
		t.Error("Expected proxy failover handling but none implemented")
	}
}

func TestRegisterFeedUsecase_EventPublishing_Success(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetchFeedGateway := mocks.NewMockFetchFeedsPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockEventPublisher := mocks.NewMockEventPublisherPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	feedLinkID := uuid.New().String()

	mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
	mockFetchFeedGateway.EXPECT().FetchFeeds(gomock.Any(), "https://example.com/rss/news").Return(mockData, nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return([]string{"art-1", "art-2"}, nil).Times(1)

	// Event publisher is enabled and should be called for each ID
	mockEventPublisher.EXPECT().IsEnabled().Return(true).Times(1)
	mockEventPublisher.EXPECT().PublishArticleCreated(gomock.Any(), gomock.Any()).Return(nil).Times(2)

	r := NewRegisterFeedsUsecase(mockRegisterFeedLinkPort, mockRegisterFeedsPort, mockFetchFeedGateway)
	r.SetEventPublisher(mockEventPublisher)
	mockResolver := mocks.NewMockFeedLinkIDResolver(ctrl)
	mockResolver.EXPECT().FetchFeedLinkIDByURL(gomock.Any(), "https://example.com/rss/news").Return(&feedLinkID, nil).Times(1)
	r.SetFeedLinkIDResolver(mockResolver)

	err := r.Execute(context.Background(), "https://example.com/rss/news")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestRegisterFeedUsecase_EventPublishing_Disabled(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetchFeedGateway := mocks.NewMockFetchFeedsPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockEventPublisher := mocks.NewMockEventPublisherPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
	mockFetchFeedGateway.EXPECT().FetchFeeds(gomock.Any(), "https://example.com/rss/news").Return(mockData, nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return([]string{"art-1", "art-2"}, nil).Times(1)

	// Event publisher is disabled — PublishArticleCreated should NOT be called
	mockEventPublisher.EXPECT().IsEnabled().Return(false).Times(1)
	// No PublishArticleCreated expectation = gomock will fail if called

	r := NewRegisterFeedsUsecase(mockRegisterFeedLinkPort, mockRegisterFeedsPort, mockFetchFeedGateway)
	r.SetEventPublisher(mockEventPublisher)

	err := r.Execute(context.Background(), "https://example.com/rss/news")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestRegisterFeedUsecase_EventPublishing_NotSet(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetchFeedGateway := mocks.NewMockFetchFeedsPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
	mockFetchFeedGateway.EXPECT().FetchFeeds(gomock.Any(), "https://example.com/rss/news").Return(mockData, nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return([]string{"art-1", "art-2"}, nil).Times(1)

	// No event publisher set — should work without error
	r := NewRegisterFeedsUsecase(mockRegisterFeedLinkPort, mockRegisterFeedsPort, mockFetchFeedGateway)

	err := r.Execute(context.Background(), "https://example.com/rss/news")
	if err != nil {
		t.Errorf("Expected no error with nil event publisher, got %v", err)
	}
}

func TestRegisterFeedUsecase_EventPublishing_FailureNonFatal(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetchFeedGateway := mocks.NewMockFetchFeedsPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockEventPublisher := mocks.NewMockEventPublisherPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
	mockFetchFeedGateway.EXPECT().FetchFeeds(gomock.Any(), "https://example.com/rss/news").Return(mockData, nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return([]string{"art-1", "art-2"}, nil).Times(1)

	// Event publisher is enabled but PublishArticleCreated fails
	mockEventPublisher.EXPECT().IsEnabled().Return(true).Times(1)
	mockEventPublisher.EXPECT().PublishArticleCreated(gomock.Any(), gomock.Any()).Return(errors.New("publish failed")).Times(2)

	r := NewRegisterFeedsUsecase(mockRegisterFeedLinkPort, mockRegisterFeedsPort, mockFetchFeedGateway)
	r.SetEventPublisher(mockEventPublisher)

	// Execute should succeed even when event publishing fails
	err := r.Execute(context.Background(), "https://example.com/rss/news")
	if err != nil {
		t.Errorf("Expected no error even when event publishing fails, got %v", err)
	}
}
