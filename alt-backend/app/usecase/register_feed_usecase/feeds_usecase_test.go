package register_feed_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/port/event_publisher_port"
	register_feed_port "alt/port/register_feed_port"
	"alt/usecase/testutil"
	"alt/utils/logger"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

// helper: build a ParsedFeed from mock feed items
func mockParsedFeed(feedLink string, items []*domain.FeedItem) *domain.ParsedFeed {
	return &domain.ParsedFeed{
		FeedLink: feedLink,
		Items:    items,
	}
}

func mockRegisterFeedResults(ids ...string) []register_feed_port.RegisterFeedResult {
	results := make([]register_feed_port.RegisterFeedResult, 0, len(ids))
	for _, id := range ids {
		results = append(results, register_feed_port.RegisterFeedResult{
			ArticleID: id,
			Created:   true,
		})
	}
	return results
}

func TestRegisterFeedUsecase_Execute(t *testing.T) {
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidateFetch := mocks.NewMockValidateAndFetchRSSPort(ctrl)
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
				pf := mockParsedFeed("https://example.com/rss/news", mockData)
				mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss/news").Return(pf, nil).Times(1)
				mockRegisterFeedLinkPort.EXPECT().RegisterFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
				mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return(mockRegisterFeedResults("id-1", "id-2"), nil).Times(1)
			},
			wantErr: false,
		},
		{
			name: "successful registration with empty feeds",
			ctx:  context.Background(),
			link: "https://example.com/rss/empty",
			mockSetup: func() {
				emptyFeeds := testutil.CreateEmptyFeedItems()
				pf := mockParsedFeed("https://example.com/rss/empty", emptyFeeds)
				mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss/empty").Return(pf, nil).Times(1)
				mockRegisterFeedLinkPort.EXPECT().RegisterFeedLink(gomock.Any(), "https://example.com/rss/empty").Return(nil).Times(1)
				mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return([]register_feed_port.RegisterFeedResult{}, nil).Times(1)
			},
			wantErr: false,
		},
		{
			name: "error validating and fetching RSS feed",
			ctx:  context.Background(),
			link: "https://example.com/rss/error",
			mockSetup: func() {
				mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss/error").Return(nil, testutil.ErrMockNetwork).Times(1)
				// Should not call other methods if validate+fetch fails
			},
			wantErr: true,
		},
		{
			name: "error registering feed link in DB",
			ctx:  context.Background(),
			link: "https://example.com/rss/db-error",
			mockSetup: func() {
				pf := mockParsedFeed("https://example.com/rss/db-error", mockData)
				mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss/db-error").Return(pf, nil).Times(1)
				mockRegisterFeedLinkPort.EXPECT().RegisterFeedLink(gomock.Any(), "https://example.com/rss/db-error").Return(testutil.ErrMockDatabase).Times(1)
				// Should not call register feeds if feed link registration fails
			},
			wantErr: true,
		},
		{
			name: "error registering feeds",
			ctx:  context.Background(),
			link: "https://example.com/rss/register-error",
			mockSetup: func() {
				pf := mockParsedFeed("https://example.com/rss/register-error", mockData)
				mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss/register-error").Return(pf, nil).Times(1)
				mockRegisterFeedLinkPort.EXPECT().RegisterFeedLink(gomock.Any(), "https://example.com/rss/register-error").Return(nil).Times(1)
				mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return(nil, testutil.ErrMockDatabase).Times(1)
			},
			wantErr: true,
		},
		{
			name: "context cancellation",
			ctx:  testutil.CreateCancelledContext(),
			link: "https://example.com/rss/cancelled",
			mockSetup: func() {
				mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss/cancelled").Return(nil, context.Canceled).Times(1)
			},
			wantErr: true,
		},
		{
			name: "invalid URL format",
			ctx:  context.Background(),
			link: "not-a-valid-url",
			mockSetup: func() {
				mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "not-a-valid-url").Return(nil, testutil.ErrMockValidation).Times(1)
			},
			wantErr: true,
		},
		{
			name: "empty URL",
			ctx:  context.Background(),
			link: "",
			mockSetup: func() {
				mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "").Return(nil, testutil.ErrMockValidation).Times(1)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()
			r := NewRegisterFeedsUsecase(mockValidateFetch, mockRegisterFeedLinkPort, mockRegisterFeedsPort, nil)
			err := r.Execute(tt.ctx, tt.link)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterFeedUsecase.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Test that ParsedFeed.FeedLink (not the input URL) is used for DB registration
func TestRegisterFeedUsecase_UsesResolvedFeedLink(t *testing.T) {
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidateFetch := mocks.NewMockValidateAndFetchRSSPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)

	// The feed's own FeedLink differs from the input URL
	pf := mockParsedFeed("https://example.com/feed.xml", testutil.CreateMockFeedItems())
	mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss").Return(pf, nil).Times(1)
	// DB registration should use the resolved FeedLink, not the input URL
	mockRegisterFeedLinkPort.EXPECT().RegisterFeedLink(gomock.Any(), "https://example.com/feed.xml").Return(nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return(mockRegisterFeedResults("id-1", "id-2"), nil).Times(1)

	r := NewRegisterFeedsUsecase(mockValidateFetch, mockRegisterFeedLinkPort, mockRegisterFeedsPort, nil)
	err := r.Execute(context.Background(), "https://example.com/rss")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestRegisterFeedUsecase_SingleFetchOnly(t *testing.T) {
	// Verify that only ONE external fetch occurs (no duplicate fetch)
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidateFetch := mocks.NewMockValidateAndFetchRSSPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)

	pf := mockParsedFeed("https://example.com/rss", testutil.CreateMockFeedItems())
	// ValidateAndFetch should be called exactly ONCE
	mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss").Return(pf, nil).Times(1)
	mockRegisterFeedLinkPort.EXPECT().RegisterFeedLink(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return(mockRegisterFeedResults("id-1", "id-2"), nil).Times(1)

	r := NewRegisterFeedsUsecase(mockValidateFetch, mockRegisterFeedLinkPort, mockRegisterFeedsPort, nil)
	err := r.Execute(context.Background(), "https://example.com/rss")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
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

	mockValidateFetch := mocks.NewMockValidateAndFetchRSSPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockSubscriptionPort := mocks.NewMockSubscriptionPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	feedLinkID := uuid.New()
	feedLinkIDStr := feedLinkID.String()
	userID := uuid.New()
	ctx := createUserContext(userID)

	pf := mockParsedFeed("https://example.com/rss/news", mockData)
	mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss/news").Return(pf, nil).Times(1)
	mockRegisterFeedLinkPort.EXPECT().RegisterFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return(mockRegisterFeedResults("id-1", "id-2"), nil).Times(1)

	// Use WaitGroup to wait for async Subscribe goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	mockSubscriptionPort.EXPECT().Subscribe(gomock.Any(), userID, feedLinkID).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
			defer wg.Done()
			return nil
		}).Times(1)

	mockResolver := mocks.NewMockFeedLinkIDResolver(ctrl)
	mockResolver.EXPECT().FetchFeedLinkIDByURL(gomock.Any(), "https://example.com/rss/news").Return(&feedLinkIDStr, nil).Times(1)

	r := NewRegisterFeedsUsecase(mockValidateFetch, mockRegisterFeedLinkPort, mockRegisterFeedsPort, &RegisterFeedsOpts{
		FeedLinkIDResolver: mockResolver,
		SubscriptionPort:   mockSubscriptionPort,
	})

	err := r.Execute(ctx, "https://example.com/rss/news")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	wg.Wait()
}

func TestRegisterFeedUsecase_AutoSubscribe_NoUserContext(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidateFetch := mocks.NewMockValidateAndFetchRSSPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockSubscriptionPort := mocks.NewMockSubscriptionPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	feedLinkID := uuid.New()
	feedLinkIDStr := feedLinkID.String()

	pf := mockParsedFeed("https://example.com/rss/news", mockData)
	mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss/news").Return(pf, nil).Times(1)
	mockRegisterFeedLinkPort.EXPECT().RegisterFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return(mockRegisterFeedResults("id-1", "id-2"), nil).Times(1)
	// Subscribe should NOT be called when there is no user context

	mockResolver := mocks.NewMockFeedLinkIDResolver(ctrl)
	mockResolver.EXPECT().FetchFeedLinkIDByURL(gomock.Any(), "https://example.com/rss/news").Return(&feedLinkIDStr, nil).Times(1)

	r := NewRegisterFeedsUsecase(mockValidateFetch, mockRegisterFeedLinkPort, mockRegisterFeedsPort, &RegisterFeedsOpts{
		FeedLinkIDResolver: mockResolver,
		SubscriptionPort:   mockSubscriptionPort,
	})

	// No user context in background context
	err := r.Execute(context.Background(), "https://example.com/rss/news")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	// Give goroutine time to run (autoSubscribeUser exits early with no user context)
	time.Sleep(50 * time.Millisecond)
}

func TestRegisterFeedUsecase_AutoSubscribe_SubscribeError(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidateFetch := mocks.NewMockValidateAndFetchRSSPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockSubscriptionPort := mocks.NewMockSubscriptionPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	feedLinkID := uuid.New()
	feedLinkIDStr := feedLinkID.String()
	userID := uuid.New()
	ctx := createUserContext(userID)

	pf := mockParsedFeed("https://example.com/rss/news", mockData)
	mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss/news").Return(pf, nil).Times(1)
	mockRegisterFeedLinkPort.EXPECT().RegisterFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return(mockRegisterFeedResults("id-1", "id-2"), nil).Times(1)

	// Subscribe fails, but Execute should still succeed
	var wg sync.WaitGroup
	wg.Add(1)
	mockSubscriptionPort.EXPECT().Subscribe(gomock.Any(), userID, feedLinkID).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
			defer wg.Done()
			return errors.New("subscription error")
		}).Times(1)

	mockResolver := mocks.NewMockFeedLinkIDResolver(ctrl)
	mockResolver.EXPECT().FetchFeedLinkIDByURL(gomock.Any(), "https://example.com/rss/news").Return(&feedLinkIDStr, nil).Times(1)

	r := NewRegisterFeedsUsecase(mockValidateFetch, mockRegisterFeedLinkPort, mockRegisterFeedsPort, &RegisterFeedsOpts{
		FeedLinkIDResolver: mockResolver,
		SubscriptionPort:   mockSubscriptionPort,
	})

	err := r.Execute(ctx, "https://example.com/rss/news")
	if err != nil {
		t.Errorf("Expected no error even when subscribe fails, got %v", err)
	}
	wg.Wait()
}

func TestRegisterFeedUsecase_InitializesAvailabilityOnSuccessfulRegistration(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidateFetch := mocks.NewMockValidateAndFetchRSSPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockAvailabilityPort := mocks.NewMockFeedLinkAvailabilityPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	pf := mockParsedFeed("https://example.com/rss/news", mockData)
	mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss/news").Return(pf, nil).Times(1)
	mockRegisterFeedLinkPort.EXPECT().RegisterFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return(mockRegisterFeedResults("id-1", "id-2"), nil).Times(1)
	mockAvailabilityPort.EXPECT().ResetFeedLinkFailures(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)

	r := NewRegisterFeedsUsecase(mockValidateFetch, mockRegisterFeedLinkPort, mockRegisterFeedsPort, &RegisterFeedsOpts{
		FeedLinkAvailability: mockAvailabilityPort,
	})

	err := r.Execute(context.Background(), "https://example.com/rss/news")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestRegisterFeedUsecase_FailsWhenAvailabilityInitializationFails(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidateFetch := mocks.NewMockValidateAndFetchRSSPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockAvailabilityPort := mocks.NewMockFeedLinkAvailabilityPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	pf := mockParsedFeed("https://example.com/rss/news", mockData)
	mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss/news").Return(pf, nil).Times(1)
	mockRegisterFeedLinkPort.EXPECT().RegisterFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return(mockRegisterFeedResults("id-1", "id-2"), nil).Times(1)
	mockAvailabilityPort.EXPECT().ResetFeedLinkFailures(gomock.Any(), "https://example.com/rss/news").Return(errors.New("availability init failed")).Times(1)

	r := NewRegisterFeedsUsecase(mockValidateFetch, mockRegisterFeedLinkPort, mockRegisterFeedsPort, &RegisterFeedsOpts{
		FeedLinkAvailability: mockAvailabilityPort,
	})

	err := r.Execute(context.Background(), "https://example.com/rss/news")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestRegisterFeedUsecase_EventPublishing_Success(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidateFetch := mocks.NewMockValidateAndFetchRSSPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockEventPublisher := mocks.NewMockEventPublisherPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	feedLinkID := uuid.New().String()

	pf := mockParsedFeed("https://example.com/rss/news", mockData)
	mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss/news").Return(pf, nil).Times(1)
	mockRegisterFeedLinkPort.EXPECT().RegisterFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return(mockRegisterFeedResults("art-1", "art-2"), nil).Times(1)

	// Event publisher is enabled and should be called for each ID (async via goroutine)
	var wg sync.WaitGroup
	wg.Add(1) // 1 for IsEnabled + publishFeedEvents goroutine completion
	mockEventPublisher.EXPECT().IsEnabled().Return(true).Times(1)
	callCount := 0
	var mu sync.Mutex
	mockEventPublisher.EXPECT().PublishArticleCreated(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ event_publisher_port.ArticleCreatedEvent) error {
			mu.Lock()
			callCount++
			if callCount == 2 {
				wg.Done()
			}
			mu.Unlock()
			return nil
		}).Times(2)

	mockResolver := mocks.NewMockFeedLinkIDResolver(ctrl)
	mockResolver.EXPECT().FetchFeedLinkIDByURL(gomock.Any(), "https://example.com/rss/news").Return(&feedLinkID, nil).Times(1)

	r := NewRegisterFeedsUsecase(mockValidateFetch, mockRegisterFeedLinkPort, mockRegisterFeedsPort, &RegisterFeedsOpts{
		EventPublisher:     mockEventPublisher,
		FeedLinkIDResolver: mockResolver,
	})

	err := r.Execute(context.Background(), "https://example.com/rss/news")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	wg.Wait()
}

func TestRegisterFeedUsecase_EventPublishing_Disabled(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidateFetch := mocks.NewMockValidateAndFetchRSSPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockEventPublisher := mocks.NewMockEventPublisherPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	pf := mockParsedFeed("https://example.com/rss/news", mockData)
	mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss/news").Return(pf, nil).Times(1)
	mockRegisterFeedLinkPort.EXPECT().RegisterFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return(mockRegisterFeedResults("art-1", "art-2"), nil).Times(1)

	// Event publisher is disabled — PublishArticleCreated should NOT be called
	mockEventPublisher.EXPECT().IsEnabled().Return(false).Times(1)

	r := NewRegisterFeedsUsecase(mockValidateFetch, mockRegisterFeedLinkPort, mockRegisterFeedsPort, &RegisterFeedsOpts{
		EventPublisher: mockEventPublisher,
	})

	err := r.Execute(context.Background(), "https://example.com/rss/news")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	// Give goroutine time to run (returns early since disabled)
	time.Sleep(50 * time.Millisecond)
}

func TestRegisterFeedUsecase_EventPublishing_NotSet(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidateFetch := mocks.NewMockValidateAndFetchRSSPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	pf := mockParsedFeed("https://example.com/rss/news", mockData)
	mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss/news").Return(pf, nil).Times(1)
	mockRegisterFeedLinkPort.EXPECT().RegisterFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return(mockRegisterFeedResults("art-1", "art-2"), nil).Times(1)

	// No event publisher set — should work without error
	r := NewRegisterFeedsUsecase(mockValidateFetch, mockRegisterFeedLinkPort, mockRegisterFeedsPort, nil)

	err := r.Execute(context.Background(), "https://example.com/rss/news")
	if err != nil {
		t.Errorf("Expected no error with nil event publisher, got %v", err)
	}
}

func TestRegisterFeedUsecase_EventPublishing_FailureNonFatal(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidateFetch := mocks.NewMockValidateAndFetchRSSPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockEventPublisher := mocks.NewMockEventPublisherPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	pf := mockParsedFeed("https://example.com/rss/news", mockData)
	mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss/news").Return(pf, nil).Times(1)
	mockRegisterFeedLinkPort.EXPECT().RegisterFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return(mockRegisterFeedResults("art-1", "art-2"), nil).Times(1)

	// Event publisher is enabled but PublishArticleCreated fails (async via goroutine)
	var wg sync.WaitGroup
	wg.Add(1)
	mockEventPublisher.EXPECT().IsEnabled().Return(true).Times(1)
	callCount := 0
	var mu sync.Mutex
	mockEventPublisher.EXPECT().PublishArticleCreated(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ event_publisher_port.ArticleCreatedEvent) error {
			mu.Lock()
			callCount++
			if callCount == 2 {
				wg.Done()
			}
			mu.Unlock()
			return errors.New("publish failed")
		}).Times(2)

	r := NewRegisterFeedsUsecase(mockValidateFetch, mockRegisterFeedLinkPort, mockRegisterFeedsPort, &RegisterFeedsOpts{
		EventPublisher: mockEventPublisher,
	})

	// Execute should succeed even when event publishing fails
	err := r.Execute(context.Background(), "https://example.com/rss/news")
	if err != nil {
		t.Errorf("Expected no error even when event publishing fails, got %v", err)
	}
	wg.Wait()
}

func TestRegisterFeedUsecase_EventPublishing_SplitsCreatedAndUpdated(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidateFetch := mocks.NewMockValidateAndFetchRSSPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)
	mockEventPublisher := mocks.NewMockEventPublisherPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	pf := mockParsedFeed("https://example.com/rss/news", mockData)
	mockValidateFetch.EXPECT().ValidateAndFetch(gomock.Any(), "https://example.com/rss/news").Return(pf, nil).Times(1)
	mockRegisterFeedLinkPort.EXPECT().RegisterFeedLink(gomock.Any(), "https://example.com/rss/news").Return(nil).Times(1)
	mockRegisterFeedsPort.EXPECT().RegisterFeeds(gomock.Any(), gomock.Any()).Return([]register_feed_port.RegisterFeedResult{
		{ArticleID: "art-1", Created: true},
		{ArticleID: "art-2", Created: false},
	}, nil).Times(1)

	var wg sync.WaitGroup
	wg.Add(1)
	mockEventPublisher.EXPECT().IsEnabled().Return(true).Times(1)
	mockEventPublisher.EXPECT().PublishArticleCreated(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mockEventPublisher.EXPECT().PublishArticleUpdated(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ event_publisher_port.ArticleUpdatedEvent) error {
			wg.Done()
			return nil
		},
	).Times(1)

	r := NewRegisterFeedsUsecase(mockValidateFetch, mockRegisterFeedLinkPort, mockRegisterFeedsPort, &RegisterFeedsOpts{
		EventPublisher: mockEventPublisher,
	})

	err := r.Execute(context.Background(), "https://example.com/rss/news")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	wg.Wait()
}
