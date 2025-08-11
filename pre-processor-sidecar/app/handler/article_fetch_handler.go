// ABOUTME: Handler layer for article fetching execution logic
// ABOUTME: Orchestrates subscription sync and article fetch processes with proper error handling

package handler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"pre-processor-sidecar/models"
	"pre-processor-sidecar/repository"
	"pre-processor-sidecar/service"
)

// ArticleFetchHandler handles article fetching orchestration
type ArticleFetchHandler struct {
	inoreaderService       *service.InoreaderService
	subscriptionSyncService *service.SubscriptionSyncService
	rateLimitManager       *service.RateLimitManager
	articleRepo           repository.ArticleRepository
	syncStateRepo         repository.SyncStateRepository
	logger                *slog.Logger
	maxArticlesPerRequest int
	maxContinuationRounds int
}

// ArticleFetchResult represents the result of an article fetch operation
type ArticleFetchResult struct {
	SubscriptionID      string    `json:"subscription_id"`
	StreamID           string    `json:"stream_id"`
	ArticlesFetched    int       `json:"articles_fetched"`
	ArticlesSaved      int       `json:"articles_saved"`
	ArticlesSkipped    int       `json:"articles_skipped"`
	ContinuationToken  string    `json:"continuation_token,omitempty"`
	HasMorePages       bool      `json:"has_more_pages"`
	ProcessingTime     time.Duration `json:"processing_time"`
	Errors             []string  `json:"errors,omitempty"`
}

// BatchFetchResult represents the result of a batch article fetch
type BatchFetchResult struct {
	SubscriptionsProcessed int                   `json:"subscriptions_processed"`
	TotalArticlesFetched   int                   `json:"total_articles_fetched"`
	TotalArticlesSaved     int                   `json:"total_articles_saved"`
	TotalArticlesSkipped   int                   `json:"total_articles_skipped"`
	TotalProcessingTime    time.Duration         `json:"total_processing_time"`
	SuccessfulFeeds        int                   `json:"successful_feeds"`
	FailedFeeds            int                   `json:"failed_feeds"`
	Results                []ArticleFetchResult  `json:"results"`
	Errors                 []string              `json:"errors,omitempty"`
}

// NewArticleFetchHandler creates a new article fetch handler
func NewArticleFetchHandler(
	inoreaderService *service.InoreaderService,
	subscriptionSyncService *service.SubscriptionSyncService,
	rateLimitManager *service.RateLimitManager,
	articleRepo repository.ArticleRepository,
	syncStateRepo repository.SyncStateRepository,
	logger *slog.Logger,
) *ArticleFetchHandler {
	if logger == nil {
		logger = slog.Default()
	}

	return &ArticleFetchHandler{
		inoreaderService:        inoreaderService,
		subscriptionSyncService: subscriptionSyncService,
		rateLimitManager:        rateLimitManager,
		articleRepo:            articleRepo,
		syncStateRepo:          syncStateRepo,
		logger:                 logger,
		maxArticlesPerRequest:  100, // Inoreader API limit
		maxContinuationRounds:  10,  // Maximum pagination rounds per subscription
	}
}

// ExecuteSubscriptionSync executes subscription synchronization
func (h *ArticleFetchHandler) ExecuteSubscriptionSync(ctx context.Context) error {
	h.logger.Info("Starting subscription synchronization execution")

	startTime := time.Now()

	// Check rate limits before proceeding
	if allowed, reason, _ := h.rateLimitManager.CheckAllowed("/subscription/list"); !allowed {
		h.logger.Warn("Subscription sync blocked by rate limiter", "reason", reason)
		return fmt.Errorf("subscription sync blocked: %s", reason)
	}

	// Execute subscription sync
	if err := h.subscriptionSyncService.SyncSubscriptionsNew(ctx); err != nil {
		h.logger.Error("Subscription synchronization failed", "error", err)
		return fmt.Errorf("subscription sync failed: %w", err)
	}

	duration := time.Since(startTime)
	h.logger.Info("Subscription synchronization completed successfully",
		"duration", duration)

	return nil
}

// ExecuteArticleFetch executes article fetching for a specific subscription
func (h *ArticleFetchHandler) ExecuteArticleFetch(ctx context.Context, streamID string) (*ArticleFetchResult, error) {
	h.logger.Info("Starting article fetch execution",
		"stream_id", streamID,
		"max_articles_per_request", h.maxArticlesPerRequest)

	startTime := time.Now()

	result := &ArticleFetchResult{
		StreamID:       streamID,
		SubscriptionID: streamID, // Will be resolved later
		Errors:         []string{},
	}

	// Get continuation token from sync state
	syncState, err := h.syncStateRepo.FindByStreamID(ctx, streamID)
	var continuationToken string
	if err != nil {
		h.logger.Debug("No previous sync state found for stream, starting fresh",
			"stream_id", streamID)
		// Create new sync state
		syncState = models.NewSyncState(streamID, "")
		if err := h.syncStateRepo.Create(ctx, syncState); err != nil {
			h.logger.Warn("Failed to create sync state", "error", err)
			// Continue without sync state
		}
	} else {
		continuationToken = syncState.ContinuationToken
		h.logger.Debug("Using existing continuation token",
			"stream_id", streamID,
			"has_token", continuationToken != "")
	}

	totalArticles := 0
	totalSaved := 0
	rounds := 0

	// Pagination rounds limit (restored to reasonable value)
	// Note: ScheduleHandler now uses ArticleFetchService with rotation, so this handler is only for direct calls

	// Fetch articles with strict pagination limits
	for rounds < h.maxContinuationRounds {
		rounds++

		// Check rate limits before each request
		endpoint := "/stream/contents/" + streamID
		if allowed, reason, remaining := h.rateLimitManager.CheckAllowed(endpoint); !allowed {
			h.logger.Warn("Article fetch blocked by rate limiter",
				"stream_id", streamID,
				"reason", reason,
				"round", rounds)
			result.Errors = append(result.Errors, fmt.Sprintf("Blocked after %d rounds: %s", rounds-1, reason))
			break
		} else {
			h.logger.Debug("Rate limit check passed",
				"stream_id", streamID,
				"remaining", remaining,
				"round", rounds)
		}

		// Fetch articles from Inoreader API
		articles, nextToken, err := h.inoreaderService.FetchStreamContents(ctx, streamID, continuationToken)
		if err != nil {
			errorMsg := fmt.Sprintf("Failed to fetch articles (round %d): %v", rounds, err)
			h.logger.Error("Article fetch failed",
				"stream_id", streamID,
				"round", rounds,
				"error", err)
			result.Errors = append(result.Errors, errorMsg)
			break
		}

		h.logger.Debug("Fetched articles from API",
			"stream_id", streamID,
			"round", rounds,
			"articles_count", len(articles),
			"has_next_token", nextToken != "")

		if len(articles) == 0 {
			h.logger.Info("No more articles available",
				"stream_id", streamID,
				"round", rounds)
			break
		}

		// Resolve subscription UUIDs for articles
		articles = h.subscriptionSyncService.ResolveArticleSubscriptionUUIDs(articles)

		// Save articles to database
		savedCount, err := h.articleRepo.CreateBatch(ctx, articles)
		if err != nil {
			errorMsg := fmt.Sprintf("Failed to save articles (round %d): %v", rounds, err)
			h.logger.Error("Article save failed",
				"stream_id", streamID,
				"round", rounds,
				"articles_count", len(articles),
				"error", err)
			result.Errors = append(result.Errors, errorMsg)
		} else {
			h.logger.Debug("Saved articles to database",
				"stream_id", streamID,
				"round", rounds,
				"saved_count", savedCount,
				"total_count", len(articles))
		}

		totalArticles += len(articles)
		totalSaved += savedCount
		continuationToken = nextToken

		// Update sync state with new continuation token
		if syncState != nil {
			syncState.UpdateContinuationToken(continuationToken)
			if err := h.syncStateRepo.Update(ctx, syncState); err != nil {
				h.logger.Warn("Failed to update sync state",
					"stream_id", streamID,
					"error", err)
			}
		}

		// Break if no more pages
		if nextToken == "" {
			h.logger.Info("Reached end of articles",
				"stream_id", streamID,
				"total_rounds", rounds)
			break
		}

		// Small delay between requests to be API-friendly
		time.Sleep(100 * time.Millisecond)
	}

	// Update final result
	result.ArticlesFetched = totalArticles
	result.ArticlesSaved = totalSaved
	result.ArticlesSkipped = totalArticles - totalSaved
	result.ContinuationToken = continuationToken
	result.HasMorePages = continuationToken != "" && rounds >= h.maxContinuationRounds
	result.ProcessingTime = time.Since(startTime)

	h.logger.Info("Article fetch execution completed",
		"stream_id", streamID,
		"articles_fetched", result.ArticlesFetched,
		"articles_saved", result.ArticlesSaved,
		"articles_skipped", result.ArticlesSkipped,
		"processing_time", result.ProcessingTime,
		"rounds", rounds,
		"errors", len(result.Errors))

	return result, nil
}

// ExecuteBatchArticleFetch executes article fetching for all subscriptions
func (h *ArticleFetchHandler) ExecuteBatchArticleFetch(ctx context.Context) (*BatchFetchResult, error) {
	h.logger.Info("Starting batch article fetch execution")

	startTime := time.Now()

	result := &BatchFetchResult{
		Results: []ArticleFetchResult{},
		Errors:  []string{},
	}

	// First ensure subscription cache is initialized
	if err := h.subscriptionSyncService.InitializeCache(ctx); err != nil {
		h.logger.Error("Failed to initialize subscription cache", "error", err)
		return nil, fmt.Errorf("cache initialization failed: %w", err)
	}

	// Get all subscriptions to process
	subscriptions, err := h.inoreaderService.FetchSubscriptions(ctx)
	if err != nil {
		h.logger.Error("Failed to fetch subscriptions for batch processing", "error", err)
		return nil, fmt.Errorf("subscription fetch failed: %w", err)
	}

	h.logger.Info("Processing subscriptions for article fetching",
		"subscription_count", len(subscriptions))

	// Restored normal batch processing (emergency limits removed)
	// Note: This method is now only used for direct calls, as ScheduleHandler uses ArticleFetchService rotation
	h.logger.Info("Processing all subscriptions in batch (emergency limits removed)",
		"total_subscriptions", len(subscriptions))

	// Process each subscription with strict rate limiting
	for i, subscription := range subscriptions {
		// Check if we should continue (rate limits, etc.)
		if allowed, reason, _ := h.rateLimitManager.CheckAllowed("/stream/contents/"); !allowed {
			errorMsg := fmt.Sprintf("Batch processing stopped due to rate limits: %s", reason)
			h.logger.Warn("Batch processing halted", "reason", reason)
			result.Errors = append(result.Errors, errorMsg)
			break
		}

		// Normal processing continues (emergency hard limits removed)

		h.logger.Info("Processing subscription in batch",
			"subscription_index", i+1,
			"total_subscriptions", len(subscriptions),
			"inoreader_id", subscription.InoreaderID,
			"title", subscription.Title)

		// Fetch articles for this subscription
		fetchResult, err := h.ExecuteArticleFetch(ctx, subscription.InoreaderID)
		if err != nil {
			errorMsg := fmt.Sprintf("Failed to fetch articles for subscription %s: %v", subscription.InoreaderID, err)
			h.logger.Error("Subscription processing failed",
				"inoreader_id", subscription.InoreaderID,
				"title", subscription.Title,
				"error", err)
			result.Errors = append(result.Errors, errorMsg)
			result.FailedFeeds++
		} else {
			h.logger.Debug("Subscription processed successfully",
				"inoreader_id", subscription.InoreaderID,
				"title", subscription.Title,
				"articles_fetched", fetchResult.ArticlesFetched,
				"articles_saved", fetchResult.ArticlesSaved)
			result.SuccessfulFeeds++
		}

		if fetchResult != nil {
			result.Results = append(result.Results, *fetchResult)
			result.TotalArticlesFetched += fetchResult.ArticlesFetched
			result.TotalArticlesSaved += fetchResult.ArticlesSaved
			result.TotalArticlesSkipped += fetchResult.ArticlesSkipped
		}

		result.SubscriptionsProcessed++

		// Standard delay between subscriptions for API courtesy (2 seconds)
		h.logger.Debug("Standard delay between subscriptions", "delay", "2s")
		time.Sleep(2 * time.Second)
	}

	result.TotalProcessingTime = time.Since(startTime)

	h.logger.Info("Batch article fetch execution completed",
		"subscriptions_processed", result.SubscriptionsProcessed,
		"successful_feeds", result.SuccessfulFeeds,
		"failed_feeds", result.FailedFeeds,
		"total_articles_fetched", result.TotalArticlesFetched,
		"total_articles_saved", result.TotalArticlesSaved,
		"total_processing_time", result.TotalProcessingTime,
		"errors", len(result.Errors))

	return result, nil
}

// ExecuteUnreadArticleFetch executes fetching of only unread articles
func (h *ArticleFetchHandler) ExecuteUnreadArticleFetch(ctx context.Context, streamID string) (*ArticleFetchResult, error) {
	h.logger.Info("Starting unread article fetch execution",
		"stream_id", streamID)

	startTime := time.Now()

	result := &ArticleFetchResult{
		StreamID:       streamID,
		SubscriptionID: streamID,
		Errors:         []string{},
	}

	// Get continuation token from sync state
	syncState, err := h.syncStateRepo.FindByStreamID(ctx, streamID)
	var continuationToken string
	if err != nil {
		h.logger.Debug("No previous sync state found for unread fetch, starting fresh",
			"stream_id", streamID)
	} else {
		continuationToken = syncState.ContinuationToken
	}

	totalArticles := 0
	totalSaved := 0
	rounds := 0

	// Fetch unread articles with pagination
	for rounds < h.maxContinuationRounds {
		rounds++

		// Check rate limits
		endpoint := "/stream/contents/" + streamID
		if allowed, reason, _ := h.rateLimitManager.CheckAllowed(endpoint); !allowed {
			h.logger.Warn("Unread article fetch blocked by rate limiter",
				"stream_id", streamID,
				"reason", reason)
			result.Errors = append(result.Errors, fmt.Sprintf("Blocked: %s", reason))
			break
		}

		// Fetch unread articles from Inoreader API
		articles, nextToken, err := h.inoreaderService.FetchUnreadStreamContents(ctx, streamID, continuationToken)
		if err != nil {
			errorMsg := fmt.Sprintf("Failed to fetch unread articles: %v", err)
			h.logger.Error("Unread article fetch failed",
				"stream_id", streamID,
				"error", err)
			result.Errors = append(result.Errors, errorMsg)
			break
		}

		if len(articles) == 0 {
			h.logger.Info("No unread articles available",
				"stream_id", streamID)
			break
		}

		// Resolve subscription UUIDs
		articles = h.subscriptionSyncService.ResolveArticleSubscriptionUUIDs(articles)

		// Save articles to database
		savedCount, err := h.articleRepo.CreateBatch(ctx, articles)
		if err != nil {
			errorMsg := fmt.Sprintf("Failed to save unread articles: %v", err)
			h.logger.Error("Unread article save failed",
				"stream_id", streamID,
				"articles_count", len(articles),
				"error", err)
			result.Errors = append(result.Errors, errorMsg)
		}

		totalArticles += len(articles)
		totalSaved += savedCount
		continuationToken = nextToken

		// Break if no more pages
		if nextToken == "" {
			break
		}

		time.Sleep(100 * time.Millisecond)
	}

	result.ArticlesFetched = totalArticles
	result.ArticlesSaved = totalSaved
	result.ArticlesSkipped = totalArticles - totalSaved
	result.ContinuationToken = continuationToken
	result.HasMorePages = continuationToken != ""
	result.ProcessingTime = time.Since(startTime)

	h.logger.Info("Unread article fetch execution completed",
		"stream_id", streamID,
		"articles_fetched", result.ArticlesFetched,
		"articles_saved", result.ArticlesSaved,
		"processing_time", result.ProcessingTime)

	return result, nil
}