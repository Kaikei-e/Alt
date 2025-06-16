package main

import (
	"context"
	"errors"
	"net/http"
	articlefetcher "pre-processor/article-fetcher"
	"pre-processor/driver"
	"pre-processor/handlers"
	"pre-processor/logger"
	"pre-processor/models"
	qualitychecker "pre-processor/quality-checker"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const BATCH_SIZE = 40
const SUMMARIZE_INTERVAL = 10 * time.Second
const FORMAT_INTERVAL = 5 * time.Minute
const QUALITY_CHECK_INTERVAL = 10 * time.Minute
const MODEL_ID = "phi4-mini:3.8b"

func main() {
	logger := logger.Init()

	ctx := context.Background()
	dbPool, err := driver.Init(ctx)
	if err != nil {
		logger.Error("Failed to initialize database connection pool", "error", err)
		panic(err)
	}

	// Initialize processors
	feedProcessor := handlers.NewFeedProcessor(dbPool, BATCH_SIZE)
	articleSummarizer := handlers.NewArticleSummarizer(dbPool, BATCH_SIZE)
	qualityChecker := handlers.NewQualityChecker(dbPool, BATCH_SIZE)

	// Run feed processing job in background
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Feed processing job panicked", "panic", r)
			}
		}()

		logger.Info("Starting feed processing job goroutine")

		for {
			logger.Info("Starting feed processing job execution")
			err := processFeedsJob(ctx, dbPool, feedProcessor)
			if err != nil {
				// Check if this is the "articles already exist" error
				if err.Error() == "articles already exist" {
					logger.Info("All articles in current batch are already fetched")
					logger.Info("Feed processing job completed, sleeping", "duration", FORMAT_INTERVAL)
					time.Sleep(FORMAT_INTERVAL)
					continue
				}

				// Check if no URLs found
				if err.Error() == "no urls found" {
					// Reset pagination to start from beginning
					feedProcessor.ResetPagination()
					logger.Info("No unprocessed feeds found, reset pagination")

					// Get stats for debugging
					stats, statsErr := feedProcessor.GetProcessingStats(ctx)
					if statsErr != nil {
						logger.Error("Failed to get processing statistics", "error", statsErr)
					} else {
						logger.Info("Processing statistics",
							"total_feeds", stats.TotalFeeds,
							"processed_feeds", stats.ProcessedFeeds,
							"remaining_feeds", stats.RemainingFeeds)
					}

					time.Sleep(FORMAT_INTERVAL)
					continue
				}

				// Handle other errors
				logger.Error("Failed to run feed processing job", "error", err)
				time.Sleep(30 * time.Second)
				continue
			}

			logger.Info("Feed processing job completed, sleeping", "duration", FORMAT_INTERVAL)
			time.Sleep(FORMAT_INTERVAL)
		}
	}()

	ch := make(chan error)
	go func() {
		ch <- healthCheckForNewsCreator()
	}()

	// Handle health check and start summarization job
	go func() {
		err := <-ch
		if err != nil {
			logger.Error("News creator is not healthy", "error", err)
			// Retry health check in a loop
			for {
				time.Sleep(30 * time.Second)
				err = healthCheckForNewsCreator()
				if err == nil {
					logger.Info("News creator is now healthy")
					break
				}
				logger.Error("News creator still not healthy", "error", err)
			}
		} else {
			logger.Info("News creator is healthy")
		}

		// Start summarization job
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Summarize job panicked", "panic", r)
			}
		}()

		for {
			logger.Info("Starting summarize job execution")
			foundArticles := summarizationJob(ctx, dbPool, articleSummarizer)
			if !foundArticles {
				logger.Info("No articles found for summarization, resetting pagination")
				articleSummarizer.ResetPagination()
			}
			logger.Info("Summarize job completed, sleeping", "duration", SUMMARIZE_INTERVAL)
			time.Sleep(SUMMARIZE_INTERVAL)
		}
	}()

	chQualityCheck := make(chan error)
	go func() {
		chQualityCheck <- healthCheckForNewsCreator()
	}()

	// Handle health check and start quality check job
	go func() {
		err := <-chQualityCheck
		if err != nil {
			logger.Error("News creator is not healthy for quality check", "error", err)
			// Retry health check in a loop
			for {
				time.Sleep(30 * time.Second)
				err = healthCheckForNewsCreator()
				if err == nil {
					logger.Info("News creator is now healthy for quality check")
					break
				}
				logger.Error("News creator still not healthy for quality check", "error", err)
			}
		} else {
			logger.Info("News creator is healthy for quality check")
		}

		// Start quality check job
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Quality check job panicked", "panic", r)
			}
		}()

		for {
			logger.Info("Starting quality check job execution")
			foundArticles := qualityCheckJob(ctx, dbPool, qualityChecker)
			if !foundArticles {
				logger.Info("No articles found for quality check, resetting pagination")
				qualityChecker.ResetPagination()
			}
			logger.Info("Quality check job completed, sleeping", "duration", QUALITY_CHECK_INTERVAL)
			time.Sleep(QUALITY_CHECK_INTERVAL)
		}
	}()

	// Keep the main goroutine alive
	select {}
}

func processFeedsJob(ctx context.Context, dbPool *pgxpool.Pool, feedProcessor *handlers.FeedProcessor) error {
	urls, hasMore, err := feedProcessor.GetNextUnprocessedFeeds(ctx)
	if err != nil {
		logger.Logger.Error("Failed to get unprocessed feeds", "error", err)
		return errors.New("failed to get unprocessed feeds")
	}

	logger.Logger.Info("Unprocessed feeds", "count", len(urls), "has_more", hasMore)

	if len(urls) == 0 {
		logger.Logger.Info("No unprocessed feeds found")
		return errors.New("no urls found")
	}

	exists, err := driver.CheckArticleExists(ctx, dbPool, urls)
	if err != nil {
		logger.Logger.Error("Failed to check article exists", "error", err)
		return errors.New("failed to check article exists")
	}

	if exists {
		logger.Logger.Info("Articles already exist for this batch")
		return errors.New("articles already exist")
	}

	for i, url := range urls {
		logger.Logger.Info("Fetching article", "url", url.String(), "index", i)
		article, err := articlefetcher.FetchArticle(url)
		if err != nil {
			logger.Logger.Error("Failed to fetch article", "error", err)
			continue
		}

		// Check if article is nil (e.g., when MP3 URLs are skipped)
		if article == nil {
			logger.Logger.Info("Article was skipped (likely MP3 or invalid content)", "url", url.String())
			continue
		}

		// Insert article to database immediately after fetching
		err = driver.CreateArticle(ctx, dbPool, article)
		if err != nil {
			logger.Logger.Error("Failed to create article", "error", err)
			return errors.New("failed to create article")
		} else {
			logger.Logger.Info("Successfully created article", "articleID", article.ID, "title", article.Title)
		}

		time.Sleep(10 * time.Second)
		logger.Logger.Info("Sleeping for 10 seconds. ", "index", i+1)
	}

	return nil
}

func summarizationJob(ctx context.Context, dbPool *pgxpool.Pool, articleSummarizer *handlers.ArticleSummarizer) bool {
	logger.Logger.Info("Starting summarization job")

	articles, hasMore, err := articleSummarizer.GetNextArticlesForSummarization(ctx)
	if err != nil {
		logger.Logger.Error("Failed to get articles for summarization", "error", err)
		return false
	}

	logger.Logger.Info("Found articles to summarize", "count", len(articles), "has_more", hasMore)

	if len(articles) == 0 {
		logger.Logger.Info("No articles found for summarization")
		return false
	}

	var processedCount, savedCount int
	for i, article := range articles {
		logger.Logger.Info("Processing article for summarization", "index", i, "articleID", article.ID, "title", article.Title)

		var articleSummary models.ArticleSummary
		summarizedContent, err := driver.ArticleSummarizerAPIClient(ctx, article)
		if err != nil {
			logger.Logger.Error("Failed to summarize article", "error", err, "articleID", article.ID)
			continue
		}

		if summarizedContent == nil {
			logger.Logger.Error("Failed to summarize article", "error", "summarized content is nil", "articleID", article.ID)
			continue
		}

		articleSummary.ArticleID = article.ID
		articleSummary.ArticleTitle = article.Title
		articleSummary.SummaryJapanese = summarizedContent.SummaryJapanese
		articleSummary.CreatedAt = time.Now()

		logger.Logger.Info("Successfully summarized article", "articleID", article.ID, "summaryLength", len(summarizedContent.SummaryJapanese))
		processedCount++

		// Save immediately to database
		logger.Logger.Info("Saving article summary to database", "articleID", article.ID)
		err = driver.CreateArticleSummary(ctx, dbPool, &articleSummary)
		if err != nil {
			logger.Logger.Error("Failed to create article summary", "error", err, "articleID", articleSummary.ArticleID)
		} else {
			logger.Logger.Info("Successfully saved article summary", "articleID", articleSummary.ArticleID)
			savedCount++
		}

		logger.Logger.Info("Sleeping for 10 seconds before next article", "currentIndex", i+1, "totalArticles", len(articles))
		time.Sleep(10 * time.Second)
	}

	logger.Logger.Info("Summarization job completed", "processedArticles", processedCount, "savedSummaries", savedCount)
	return true
}

func qualityCheckJob(ctx context.Context, dbPool *pgxpool.Pool, qualityChecker *handlers.QualityChecker) bool {
	logger.Logger.Info("Starting quality check job")

	// Fetch articles and summaries
	articleWithSummaries, hasMore, err := qualityChecker.GetNextArticlesForQualityCheck(ctx)
	if err != nil {
		logger.Logger.Error("Failed to fetch articles with summaries", "error", err)
		return false
	}

	if len(articleWithSummaries) == 0 {
		logger.Logger.Info("No articles found for quality check")
		return false
	}

	logger.Logger.Info("Found articles for quality check", "count", len(articleWithSummaries), "has_more", hasMore)

	// Process each article with quality scoring
	processedCount := 0
	successCount := 0
	errorCount := 0
	for i, articleWithSummary := range articleWithSummaries {
		logger.Logger.Info("Processing article for quality check", "index", i, "articleID", articleWithSummary.ArticleID)

		err = qualitychecker.RemoveLowScoreSummary(ctx, dbPool, &articleWithSummary)
		if err != nil {
			logger.Logger.Error("Failed to process article quality check", "error", err, "articleID", articleWithSummary.ArticleID)
			errorCount++
			continue
		}

		processedCount++
		successCount++
		logger.Logger.Info("Successfully processed article quality check", "articleID", articleWithSummary.ArticleID)
		logger.Logger.Info("Sleeping for 30 seconds before next article", "currentIndex", i+1, "totalArticles", len(articleWithSummaries))
		time.Sleep(30 * time.Second)
	}

	logger.Logger.Info("Quality check completed",
		"processedArticles", processedCount,
		"successfulArticles", successCount,
		"errorArticles", errorCount,
		"totalArticles", len(articleWithSummaries))

	return true
}

func healthCheckForNewsCreator() error {
	resp, err := http.Get("http://news-creator:11434/api/tags")
	if err != nil {
		logger.Logger.Error("Failed to send request to news creator", "error", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Logger.Error("News creator is not healthy", "status", resp.StatusCode)
		return errors.New("news creator is not healthy")
	}

	return nil
}
