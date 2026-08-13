package service

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"pre-processor-sidecar/mocks"
	"pre-processor-sidecar/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// Use the generated mocks from mocks package

// MockTokenService implements a simple token service for testing
type MockTokenService struct {
	token string
	valid bool
}

func (m *MockTokenService) GetCurrentToken(ctx context.Context) (string, error) {
	if m.token == "" {
		return "", fmt.Errorf("no token available")
	}
	return m.token, nil
}

func (m *MockTokenService) IsTokenValid(ctx context.Context) (bool, error) {
	return m.valid, nil
}

func (m *MockTokenService) RefreshToken(ctx context.Context) error {
	return nil // Mock refresh always succeeds
}

// setupSubscriptionMock sets up a standard subscription mock for testing
func setupSubscriptionMock(subscriptionRepo *mocks.MockSubscriptionRepository) {
	subscriptionRepo.EXPECT().
		GetAllSubscriptions(gomock.Any()).
		Return([]models.InoreaderSubscription{
			{
				DatabaseID:  uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
				InoreaderID: "feed/http://example.com/rss",
				URL:         "http://example.com/rss",
				Title:       "Example Feed",
			},
		}, nil).AnyTimes()
}

// Tests use proper gomock generated mocks

// tier1TestArticle builds an article long enough to survive the Tier1 filter.
func tier1TestArticle(itemID, originStreamID string) *models.Article {
	return &models.Article{
		ID:             uuid.New(),
		InoreaderID:    "tag:google.com,2005:reader/item/" + itemID,
		ArticleURL:     "https://example.com/" + itemID,
		Title:          "Test Article " + itemID,
		Content:        strings.Repeat("full article body sentence. ", 40),
		ContentLength:  1000,
		ContentType:    "html",
		OriginStreamID: originStreamID,
	}
}

type stubTokenProvider struct{}

func (stubTokenProvider) GetValidToken(ctx context.Context) (*models.OAuth2Token, error) {
	return &models.OAuth2Token{AccessToken: "test-access-token"}, nil
}

func (stubTokenProvider) EnsureValidToken(ctx context.Context) (*models.OAuth2Token, error) {
	return &models.OAuth2Token{AccessToken: "test-access-token"}, nil
}

func TestFetchArticles_PartialBatchFailure_DoesNotAdvanceContinuationToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const streamID = "feed/http://example.com/rss"
	const currentToken = "token-page-1"

	articles := []*models.Article{
		tier1TestArticle("aaaa1111", streamID),
		tier1TestArticle("bbbb2222", streamID),
	}

	client := mocks.NewMockInoreaderClient(ctrl)
	client.EXPECT().
		FetchStreamContents(gomock.Any(), gomock.Any(), streamID, currentToken, gomock.Any()).
		Return(map[string]interface{}{}, nil)
	client.EXPECT().
		ParseStreamContentsResponse(gomock.Any()).
		Return(articles, "token-page-2", nil)

	articleRepo := mocks.NewMockArticleRepository(ctrl)
	articleRepo.EXPECT().
		CreateBatch(gomock.Any(), gomock.Any()).
		Return(1, 1, nil) // one of the two articles was not persisted

	staleSync := time.Now().Add(-2 * time.Hour)

	syncStateRepo := mocks.NewMockSyncStateRepository(ctrl)
	syncStateRepo.EXPECT().
		FindByStreamID(gomock.Any(), streamID).
		Return(&models.SyncState{StreamID: streamID, ContinuationToken: currentToken, LastSync: staleSync}, nil)

	var persisted *models.SyncState
	syncStateRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, state *models.SyncState) error {
			persisted = state
			return nil
		})

	subscriptionRepo := mocks.NewMockSubscriptionRepository(ctrl)
	setupSubscriptionMock(subscriptionRepo)

	inoreaderService := NewInoreaderService(client, nil, stubTokenProvider{}, slog.Default())
	svc := NewArticleFetchService(inoreaderService, articleRepo, syncStateRepo, subscriptionRepo, slog.Default())

	result, err := svc.FetchArticles(context.Background(), streamID, 100)
	require.NoError(t, err)
	assert.Equal(t, currentToken, result.ContinuationToken,
		"a page with unpersisted articles must be refetched, not skipped")

	require.NotNil(t, persisted, "sync state must still be persisted so the stream rotates out")
	assert.Equal(t, currentToken, persisted.ContinuationToken,
		"the held token must be written back unchanged")
	assert.True(t, persisted.LastSync.After(staleSync),
		"last_sync must advance or the scheduler re-selects this stream forever")
}

func TestFetchArticles_TotalBatchFailure_PersistsSyncStateAndReportsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const streamID = "feed/http://example.com/rss"
	const currentToken = "token-page-1"

	articles := []*models.Article{
		tier1TestArticle("aaaa1111", streamID),
		tier1TestArticle("bbbb2222", streamID),
	}

	client := mocks.NewMockInoreaderClient(ctrl)
	client.EXPECT().
		FetchStreamContents(gomock.Any(), gomock.Any(), streamID, currentToken, gomock.Any()).
		Return(map[string]interface{}{}, nil)
	client.EXPECT().
		ParseStreamContentsResponse(gomock.Any()).
		Return(articles, "token-page-2", nil)

	batchErr := fmt.Errorf("all articles failed to upsert, last error: %w", fmt.Errorf("connection reset"))

	articleRepo := mocks.NewMockArticleRepository(ctrl)
	articleRepo.EXPECT().
		CreateBatch(gomock.Any(), gomock.Any()).
		Return(0, len(articles), batchErr) // whole page lost

	staleSync := time.Now().Add(-2 * time.Hour)

	syncStateRepo := mocks.NewMockSyncStateRepository(ctrl)
	syncStateRepo.EXPECT().
		FindByStreamID(gomock.Any(), streamID).
		Return(&models.SyncState{StreamID: streamID, ContinuationToken: currentToken, LastSync: staleSync}, nil)

	var persisted *models.SyncState
	syncStateRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, state *models.SyncState) error {
			persisted = state
			return nil
		})

	subscriptionRepo := mocks.NewMockSubscriptionRepository(ctrl)
	setupSubscriptionMock(subscriptionRepo)

	inoreaderService := NewInoreaderService(client, nil, stubTokenProvider{}, slog.Default())
	svc := NewArticleFetchService(inoreaderService, articleRepo, syncStateRepo, subscriptionRepo, slog.Default())

	_, err := svc.FetchArticles(context.Background(), streamID, 100)
	require.Error(t, err, "a total batch failure must still be surfaced to the caller")
	assert.ErrorIs(t, err, batchErr)

	require.NotNil(t, persisted, "sync state must be persisted so the stream rotates out of the oldest-last_sync slot")
	assert.Equal(t, currentToken, persisted.ContinuationToken,
		"the failed page must be refetched, so the token must be written back unchanged")
	assert.True(t, persisted.LastSync.After(staleSync),
		"last_sync must advance or the scheduler re-selects this stream forever and starves every other stream")
}

func TestFetchArticles_StreamFetchFailure_HoldsTokenAndAdvancesLastSync(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const streamID = "feed/http://stale.example.com/rss"
	const currentToken = "token-page-1"

	client := mocks.NewMockInoreaderClient(ctrl)
	client.EXPECT().
		FetchStreamContents(gomock.Any(), gomock.Any(), streamID, currentToken, gomock.Any()).
		Return(nil, fmt.Errorf("API request failed with status 404"))

	articleRepo := mocks.NewMockArticleRepository(ctrl)

	staleSync := time.Now().Add(-2 * time.Hour)

	syncStateRepo := mocks.NewMockSyncStateRepository(ctrl)
	syncStateRepo.EXPECT().
		FindByStreamID(gomock.Any(), streamID).
		Return(&models.SyncState{StreamID: streamID, ContinuationToken: currentToken, LastSync: staleSync}, nil)

	var persisted *models.SyncState
	syncStateRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, state *models.SyncState) error {
			persisted = state
			return nil
		})

	subscriptionRepo := mocks.NewMockSubscriptionRepository(ctrl)
	setupSubscriptionMock(subscriptionRepo)

	inoreaderService := NewInoreaderService(client, nil, stubTokenProvider{}, slog.Default())
	svc := NewArticleFetchService(inoreaderService, articleRepo, syncStateRepo, subscriptionRepo, slog.Default())

	_, err := svc.FetchArticles(context.Background(), streamID, 100)
	require.Error(t, err, "a failed fetch must still be surfaced to the caller")

	require.NotNil(t, persisted, "sync state must be persisted so a permanently failing stream rotates out")
	assert.Equal(t, currentToken, persisted.ContinuationToken,
		"nothing was fetched, so the token must be written back unchanged")
	assert.True(t, persisted.LastSync.After(staleSync),
		"last_sync must advance or one stale stream_id starves every other stream")
}

func TestFetchArticles_UnresolvableArticles_AreDroppedNotFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const streamID = "feed/http://example.com/rss"
	const currentToken = "token-page-1"
	const nextToken = "token-page-2"

	healthy := tier1TestArticle("aaaa1111", streamID)

	// Auto-creation of its subscription fails below, so this one keeps uuid.Nil.
	orphan := tier1TestArticle("bbbb2222", "feed/http://orphan.example.com/rss")

	urlless := tier1TestArticle("cccc3333", streamID)
	urlless.ArticleURL = ""

	client := mocks.NewMockInoreaderClient(ctrl)
	client.EXPECT().
		FetchStreamContents(gomock.Any(), gomock.Any(), streamID, currentToken, gomock.Any()).
		Return(map[string]interface{}{}, nil)
	client.EXPECT().
		ParseStreamContentsResponse(gomock.Any()).
		Return([]*models.Article{healthy, orphan, urlless}, nextToken, nil)

	var batched []*models.Article
	articleRepo := mocks.NewMockArticleRepository(ctrl)
	articleRepo.EXPECT().
		CreateBatch(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, articles []*models.Article) (int, int, error) {
			batched = articles
			return len(articles), 0, nil
		})

	syncStateRepo := mocks.NewMockSyncStateRepository(ctrl)
	syncStateRepo.EXPECT().
		FindByStreamID(gomock.Any(), streamID).
		Return(&models.SyncState{StreamID: streamID, ContinuationToken: currentToken}, nil)

	var persisted *models.SyncState
	syncStateRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, state *models.SyncState) error {
			persisted = state
			return nil
		})

	subscriptionRepo := mocks.NewMockSubscriptionRepository(ctrl)
	setupSubscriptionMock(subscriptionRepo)
	subscriptionRepo.EXPECT().
		CreateSubscription(gomock.Any(), gomock.Any()).
		Return(fmt.Errorf("subscription insert rejected"))

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

	inoreaderService := NewInoreaderService(client, nil, stubTokenProvider{}, logger)
	svc := NewArticleFetchService(inoreaderService, articleRepo, syncStateRepo, subscriptionRepo, logger)

	result, err := svc.FetchArticles(context.Background(), streamID, 100)
	require.NoError(t, err)

	require.Len(t, batched, 1, "only the persistable article may reach the repository")
	assert.Equal(t, healthy.InoreaderID, batched[0].InoreaderID)

	assert.Equal(t, 1, result.NewArticles)
	assert.Equal(t, 2, result.DroppedUnresolvable,
		"permanently dropped articles must be visible in the result, not folded into success")
	assert.Equal(t, nextToken, result.ContinuationToken,
		"a page whose only losses are unresolvable must not hold the token forever")
	assert.Empty(t, result.Errors,
		"a dropped article is not a retryable failure, so nothing may hold the token")
	require.NotNil(t, persisted)
	assert.Equal(t, nextToken, persisted.ContinuationToken)

	logged := logs.String()
	assert.Contains(t, logged, "nil_subscription_id")
	assert.Contains(t, logged, "empty_article_url")
	assert.Contains(t, logged, orphan.InoreaderID)
	assert.Contains(t, logged, urlless.InoreaderID)
}

func TestFetchArticles_FullBatchSuccess_AdvancesContinuationToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const streamID = "feed/http://example.com/rss"
	const currentToken = "token-page-1"
	const nextToken = "token-page-2"

	articles := []*models.Article{
		tier1TestArticle("aaaa1111", streamID),
		tier1TestArticle("bbbb2222", streamID),
	}

	client := mocks.NewMockInoreaderClient(ctrl)
	client.EXPECT().
		FetchStreamContents(gomock.Any(), gomock.Any(), streamID, currentToken, gomock.Any()).
		Return(map[string]interface{}{}, nil)
	client.EXPECT().
		ParseStreamContentsResponse(gomock.Any()).
		Return(articles, nextToken, nil)

	articleRepo := mocks.NewMockArticleRepository(ctrl)
	articleRepo.EXPECT().
		CreateBatch(gomock.Any(), gomock.Any()).
		Return(2, 0, nil)

	syncStateRepo := mocks.NewMockSyncStateRepository(ctrl)
	syncStateRepo.EXPECT().
		FindByStreamID(gomock.Any(), streamID).
		Return(&models.SyncState{StreamID: streamID, ContinuationToken: currentToken}, nil)
	syncStateRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, state *models.SyncState) error {
			assert.Equal(t, nextToken, state.ContinuationToken)
			return nil
		})

	subscriptionRepo := mocks.NewMockSubscriptionRepository(ctrl)
	setupSubscriptionMock(subscriptionRepo)

	inoreaderService := NewInoreaderService(client, nil, stubTokenProvider{}, slog.Default())
	svc := NewArticleFetchService(inoreaderService, articleRepo, syncStateRepo, subscriptionRepo, slog.Default())

	result, err := svc.FetchArticles(context.Background(), streamID, 100)
	require.NoError(t, err)
	assert.Equal(t, nextToken, result.ContinuationToken)
}
