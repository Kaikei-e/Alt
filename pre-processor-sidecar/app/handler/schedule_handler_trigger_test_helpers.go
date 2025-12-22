package handler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"pre-processor-sidecar/models"
	"pre-processor-sidecar/repository"
	"pre-processor-sidecar/service"

	"github.com/google/uuid"
)

func newScheduleHandlerForTriggerTests(t *testing.T) *ScheduleHandler {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tokenService, err := service.NewSimpleTokenService(service.SimpleTokenConfig{
		ClientID:            "test-client",
		ClientSecret:        "test-secret",
		InitialAccessToken:  "test-access-token",
		InitialRefreshToken: "test-refresh-token",
		BaseURL:             "http://example.invalid",
	}, &fakeOAuth2TokenRepo{}, logger)
	if err != nil {
		t.Fatalf("failed to create token service: %v", err)
	}

	apiUsageRepo := &fakeAPIUsageRepo{}
	inoreaderService := service.NewInoreaderService(&fakeInoreaderClient{}, apiUsageRepo, tokenService, logger)

	subscriptionRepo := &fakeSubscriptionRepo{}
	syncRepo := &fakeSyncStateRepo{}
	subscriptionSyncService := service.NewSubscriptionSyncService(inoreaderService, subscriptionRepo, syncRepo, logger)

	rateLimitManager := service.NewRateLimitManager(apiUsageRepo, logger)
	articleFetchHandler := NewArticleFetchHandler(inoreaderService, subscriptionSyncService, rateLimitManager, nil, nil, logger)

	articleFetchService := service.NewArticleFetchService(
		inoreaderService,
		&fakeArticleRepo{},
		syncRepo,
		subscriptionRepo,
		logger,
	)

	return NewScheduleHandler(articleFetchHandler, articleFetchService, logger)
}

type fakeAPIUsageRepo struct{}

func (f *fakeAPIUsageRepo) GetTodaysUsage(ctx context.Context) (*models.APIUsageTracking, error) {
	return models.NewAPIUsageTracking(), nil
}

func (f *fakeAPIUsageRepo) CreateUsageRecord(ctx context.Context, usage *models.APIUsageTracking) error {
	return nil
}

func (f *fakeAPIUsageRepo) UpdateUsageRecord(ctx context.Context, usage *models.APIUsageTracking) error {
	return nil
}

type fakeOAuth2TokenRepo struct{}

func (f *fakeOAuth2TokenRepo) GetCurrentToken(ctx context.Context) (*models.OAuth2Token, error) {
	return nil, repository.ErrTokenNotFound
}

func (f *fakeOAuth2TokenRepo) SaveToken(ctx context.Context, token *models.OAuth2Token) error {
	return nil
}

func (f *fakeOAuth2TokenRepo) UpdateToken(ctx context.Context, token *models.OAuth2Token) error {
	return nil
}

func (f *fakeOAuth2TokenRepo) DeleteToken(ctx context.Context) error {
	return nil
}

type fakeInoreaderClient struct{}

func (f *fakeInoreaderClient) FetchSubscriptionList(ctx context.Context, accessToken string) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (f *fakeInoreaderClient) FetchStreamContents(ctx context.Context, accessToken, streamID, continuationToken string, maxArticles int) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (f *fakeInoreaderClient) FetchUnreadStreamContents(ctx context.Context, accessToken, streamID, continuationToken string, maxArticles int) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (f *fakeInoreaderClient) RefreshToken(ctx context.Context, refreshToken string) (*models.InoreaderTokenResponse, error) {
	return &models.InoreaderTokenResponse{}, nil
}

func (f *fakeInoreaderClient) ValidateToken(ctx context.Context, accessToken string) (bool, error) {
	return true, nil
}

func (f *fakeInoreaderClient) MakeAuthenticatedRequestWithHeaders(ctx context.Context, accessToken, endpoint string, params map[string]string) (map[string]interface{}, map[string]string, error) {
	return map[string]interface{}{}, map[string]string{}, nil
}

func (f *fakeInoreaderClient) ParseSubscriptionsResponse(response map[string]interface{}) ([]*models.Subscription, error) {
	return []*models.Subscription{}, nil
}

func (f *fakeInoreaderClient) ParseStreamContentsResponse(response map[string]interface{}) ([]*models.Article, string, error) {
	return []*models.Article{}, "", nil
}

type fakeSubscriptionRepo struct {
	subscriptions []models.InoreaderSubscription
}

func (f *fakeSubscriptionRepo) SaveSubscriptions(ctx context.Context, subscriptions []models.InoreaderSubscription) error {
	f.subscriptions = append([]models.InoreaderSubscription{}, subscriptions...)
	return nil
}

func (f *fakeSubscriptionRepo) GetAllSubscriptions(ctx context.Context) ([]models.InoreaderSubscription, error) {
	return append([]models.InoreaderSubscription{}, f.subscriptions...), nil
}

func (f *fakeSubscriptionRepo) GetAll(ctx context.Context) ([]models.InoreaderSubscription, error) {
	return f.GetAllSubscriptions(ctx)
}

func (f *fakeSubscriptionRepo) FindByID(ctx context.Context, id uuid.UUID) (*models.InoreaderSubscription, error) {
	return nil, errors.New("subscription not found")
}

func (f *fakeSubscriptionRepo) UpdateSubscription(ctx context.Context, subscription models.InoreaderSubscription) error {
	return nil
}

func (f *fakeSubscriptionRepo) DeleteSubscription(ctx context.Context, inoreaderID string) error {
	return nil
}

func (f *fakeSubscriptionRepo) CreateSubscription(ctx context.Context, subscription *models.Subscription) error {
	return nil
}

type fakeSyncStateRepo struct{}

func (f *fakeSyncStateRepo) Create(ctx context.Context, syncState *models.SyncState) error {
	return nil
}

func (f *fakeSyncStateRepo) FindByStreamID(ctx context.Context, streamID string) (*models.SyncState, error) {
	return nil, errors.New("sync state not found")
}

func (f *fakeSyncStateRepo) FindByID(ctx context.Context, id uuid.UUID) (*models.SyncState, error) {
	return nil, errors.New("sync state not found")
}

func (f *fakeSyncStateRepo) GetAll(ctx context.Context) ([]*models.SyncState, error) {
	return nil, nil
}

func (f *fakeSyncStateRepo) GetStaleStates(ctx context.Context, olderThan time.Time) ([]*models.SyncState, error) {
	return nil, nil
}

func (f *fakeSyncStateRepo) GetOldestOne(ctx context.Context) (*models.SyncState, error) {
	return nil, errors.New("sync state not found")
}

func (f *fakeSyncStateRepo) Update(ctx context.Context, syncState *models.SyncState) error {
	return nil
}

func (f *fakeSyncStateRepo) UpdateContinuationToken(ctx context.Context, streamID, token string) error {
	return nil
}

func (f *fakeSyncStateRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (f *fakeSyncStateRepo) DeleteByStreamID(ctx context.Context, streamID string) error {
	return nil
}

func (f *fakeSyncStateRepo) DeleteStale(ctx context.Context, olderThan time.Time) (int, error) {
	return 0, nil
}

func (f *fakeSyncStateRepo) CleanupStale(ctx context.Context, retentionDays int) (int, error) {
	return 0, nil
}

type fakeArticleRepo struct{}

func (f *fakeArticleRepo) FindByInoreaderID(ctx context.Context, inoreaderID string) (*models.Article, error) {
	return nil, errors.New("article not found")
}

func (f *fakeArticleRepo) Create(ctx context.Context, article *models.Article) error {
	return nil
}

func (f *fakeArticleRepo) CreateBatch(ctx context.Context, articles []*models.Article) (int, error) {
	return len(articles), nil
}

func (f *fakeArticleRepo) Update(ctx context.Context, article *models.Article) error {
	return nil
}

func (f *fakeArticleRepo) GetUnprocessed(ctx context.Context, limit int) ([]*models.Article, error) {
	return nil, nil
}

func (f *fakeArticleRepo) MarkAsProcessed(ctx context.Context, articleID string) error {
	return nil
}

func (f *fakeArticleRepo) DeleteOld(ctx context.Context, olderThan time.Time) (int, error) {
	return 0, nil
}
