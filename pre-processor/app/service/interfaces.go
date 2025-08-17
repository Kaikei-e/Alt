package service

import (
	"context"

	"pre-processor/models"
)

//go:generate mockgen -source=interfaces.go -destination=../test/mocks/service_mocks.go -package=mocks

// FeedProcessorService handles RSS feed processing business logic.
type FeedProcessorService interface {
	ProcessFeeds(ctx context.Context, batchSize int) (*ProcessingResult, error)
	GetProcessingStats(ctx context.Context) (*ProcessingStats, error)
	ResetPagination() error
}

// ArticleSummarizerService handles article summarization business logic.
type ArticleSummarizerService interface {
	SummarizeArticles(ctx context.Context, batchSize int) (*SummarizationResult, error)
	HasUnsummarizedArticles(ctx context.Context) (bool, error)
	ResetPagination() error
}

// QualityCheckerService handles article quality checking business logic.
type QualityCheckerService interface {
	CheckQuality(ctx context.Context, batchSize int) (*QualityResult, error)
	ProcessLowQualityArticles(ctx context.Context, articles []models.ArticleWithSummary) error
	ResetPagination() error
}

// ArticleFetcherService handles external article fetching.
type ArticleFetcherService interface {
	FetchArticle(ctx context.Context, url string) (*models.Article, error)
	ValidateURL(url string) error
}

// HealthCheckerService handles health checking for external services.
type HealthCheckerService interface {
	CheckNewsCreatorHealth(ctx context.Context) error
	WaitForHealthy(ctx context.Context) error
}

// ProcessingResult represents the result of feed processing.
type ProcessingResult struct {
	Errors         []error
	ProcessedCount int
	SuccessCount   int
	ErrorCount     int
	HasMore        bool
}

// SummarizationResult represents the result of article summarization.
type SummarizationResult struct {
	Errors         []error
	ProcessedCount int
	SuccessCount   int
	ErrorCount     int
	HasMore        bool
}

// QualityResult represents the result of quality checking.
type QualityResult struct {
	Errors         []error
	ProcessedCount int
	SuccessCount   int
	ErrorCount     int
	RemovedCount   int
	RetainedCount  int
	HasMore        bool
}

// ProcessingStats represents processing statistics.
type ProcessingStats struct {
	TotalFeeds     int
	ProcessedFeeds int
	RemainingFeeds int
}

// UserSyncService handles user subscription synchronization business logic.
type UserSyncService interface {
	SyncUserSubscriptions(ctx context.Context) error
	GetUserSubscriptions(ctx context.Context) ([]Subscription, error)
}

// InoreaderClient handles external Inoreader API communication.
type InoreaderClient interface {
	GetUserSubscriptions(ctx context.Context, userID string) ([]Subscription, error)
}

// UserSubscriptionRepository handles user subscription data persistence.
type UserSubscriptionRepository interface {
	SaveUserSubscriptions(ctx context.Context, tenantID, userID string, subscriptions []Subscription) error
}

// Subscription represents a user's RSS feed subscription.
type Subscription struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}
