package handlers

import (
	"context"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"pre-processor/driver"
	"pre-processor/models"
	logger "pre-processor/utils/logger"
)

// ProcessingStats represents current processing statistics.
type ProcessingStats struct {
	TotalFeeds     int
	ProcessedFeeds int
	RemainingFeeds int
}

// FeedCursor represents the cursor state for feed pagination.
type FeedCursor struct {
	LastCreatedAt *time.Time
	LastID        string
}

// ArticleCursor represents the cursor state for article pagination.
type ArticleCursor struct {
	LastCreatedAt *time.Time
	LastID        string
}

// FeedProcessor handles feed processing with cursor-based pagination.
type FeedProcessor struct {
	cursor    *FeedCursor
	db        *pgxpool.Pool
	batchSize int
}

// NewFeedProcessor creates a new feed processor.
func NewFeedProcessor(db *pgxpool.Pool, batchSize int) *FeedProcessor {
	return &FeedProcessor{
		cursor:    &FeedCursor{},
		batchSize: batchSize,
		db:        db,
	}
}

// GetNextUnprocessedFeeds gets the next batch of unprocessed feeds using cursor-based pagination.
func (fp *FeedProcessor) GetNextUnprocessedFeeds(ctx context.Context) ([]url.URL, bool, error) {
	// Use the new cursor-based driver function that returns cursor information
	feedUrls, lastCreatedAt, lastID, err := driver.GetSourceURLs(fp.cursor.LastCreatedAt, fp.cursor.LastID, ctx, fp.db)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to get source URLs", "error", err)
		return nil, false, err
	}

	hasMore := len(feedUrls) == 40 // Has more if we got a full batch (hardcoded limit in driver)

	// Update cursor for next batch using the returned cursor info
	if len(feedUrls) > 0 && lastCreatedAt != nil {
		fp.cursor.LastCreatedAt = lastCreatedAt
		fp.cursor.LastID = lastID
	}

	logger.Logger.InfoContext(ctx, "Got unprocessed feeds", "count", len(feedUrls), "has_more", hasMore)

	return feedUrls, hasMore, nil
}

// ResetPagination resets the pagination to start from the beginning.
func (fp *FeedProcessor) ResetPagination() {
	fp.cursor = &FeedCursor{}

	logger.Logger.Info("Feed processor pagination reset")
}

// GetProcessingStats gets current processing statistics.
func (fp *FeedProcessor) GetProcessingStats(ctx context.Context) (*ProcessingStats, error) {
	totalFeeds, processedFeeds, err := driver.GetFeedStatistics(ctx, fp.db)
	if err != nil {
		return nil, err
	}

	return &ProcessingStats{
		TotalFeeds:     totalFeeds,
		ProcessedFeeds: processedFeeds,
		RemainingFeeds: totalFeeds - processedFeeds,
	}, nil
}

// ArticleSummarizer handles article summarization with cursor-based pagination.
type ArticleSummarizer struct {
	cursor    *ArticleCursor
	db        *pgxpool.Pool
	batchSize int
}

// NewArticleSummarizer creates a new article summarizer.
func NewArticleSummarizer(db *pgxpool.Pool, batchSize int) *ArticleSummarizer {
	return &ArticleSummarizer{
		cursor:    &ArticleCursor{},
		batchSize: batchSize,
		db:        db,
	}
}

// GetNextArticlesForSummarization gets the next batch of articles that need summarization.
func (as *ArticleSummarizer) GetNextArticlesForSummarization(ctx context.Context) ([]*models.Article, bool, error) {
	// Use the new cursor-based driver function that returns cursor information
	articles, lastCreatedAt, lastID, err := driver.GetArticlesForSummarization(ctx, as.db, as.cursor.LastCreatedAt, as.cursor.LastID, as.batchSize)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to get articles for summarization", "error", err)
		return nil, false, err
	}

	hasMore := len(articles) == as.batchSize

	// Update cursor for next batch using the returned cursor info
	if len(articles) > 0 && lastCreatedAt != nil {
		as.cursor.LastCreatedAt = lastCreatedAt
		as.cursor.LastID = lastID
	}

	logger.Logger.InfoContext(ctx, "Got articles for summarization", "count", len(articles), "has_more", hasMore)

	return articles, hasMore, nil
}

// ResetPagination resets the pagination to start from the beginning.
func (as *ArticleSummarizer) ResetPagination() {
	as.cursor = &ArticleCursor{}

	logger.Logger.Info("Article summarizer pagination reset")
}

// HasUnsummarizedArticles checks if there are articles without summaries.
func (as *ArticleSummarizer) HasUnsummarizedArticles(ctx context.Context) (bool, error) {
	// Use the new efficient driver function
	return driver.HasUnsummarizedArticles(ctx, as.db)
}

// QualityChecker handles quality checking with cursor-based pagination.
type QualityChecker struct {
	cursor    *ArticleCursor
	db        *pgxpool.Pool
	batchSize int
}

// NewQualityChecker creates a new quality checker.
func NewQualityChecker(db *pgxpool.Pool, batchSize int) *QualityChecker {
	return &QualityChecker{
		cursor:    &ArticleCursor{},
		batchSize: batchSize,
		db:        db,
	}
}

// GetNextArticlesForQualityCheck gets the next batch of articles for quality checking using cursor-based pagination.
func (qc *QualityChecker) GetNextArticlesForQualityCheck(ctx context.Context) ([]driver.ArticleWithSummary, bool, error) {
	// Use the new consolidated cursor-based driver function
	articles, lastCreatedAt, lastID, err := driver.GetArticlesWithSummaries(ctx, qc.db, qc.cursor.LastCreatedAt, qc.cursor.LastID, qc.batchSize)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to get articles for quality check", "error", err)
		return nil, false, err
	}

	hasMore := len(articles) == qc.batchSize

	// Update cursor for next batch using the returned cursor info
	if len(articles) > 0 && lastCreatedAt != nil {
		qc.cursor.LastCreatedAt = lastCreatedAt
		qc.cursor.LastID = lastID
	}

	logger.Logger.InfoContext(ctx, "Got articles for quality check", "count", len(articles), "has_more", hasMore)

	return articles, hasMore, nil
}

// ResetPagination resets the pagination to start from the beginning.
func (qc *QualityChecker) ResetPagination() {
	qc.cursor = &ArticleCursor{}

	logger.Logger.Info("Quality checker pagination reset")
}
