package main

import (
	"context"
	"errors"
	"net/http"
	articlefetcher "pre-processor/article-fetcher"
	"pre-processor/driver"
	"pre-processor/logger"
	"pre-processor/models"
	qualitychecker "pre-processor/quality-checker"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const OFFSET_STEP = 40
const SUMMARIZE_INTERVAL = 20 * time.Second
const FORMAT_INTERVAL = 10 * time.Minute
const MODEL_ID = "phi4-mini:3.8b"

func main() {
	logger := logger.Init()

	ctx := context.Background()
	dbPool, err := driver.Init(ctx)
	if err != nil {
		logger.Error("Failed to initialize database connection pool", "error", err)
		panic(err)
	}

	// Run job in background. The job will run every 30 minutes.
	var offset int
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Format job panicked", "panic", r)
			}
		}()

		logger.Info("Starting format job goroutine")
		for {
			logger.Info("Starting format job execution", "offset", offset)
			err := job_for_format(offset, ctx, dbPool)
			if err != nil {
				// Check if this is the "articles already exist" error
				if err.Error() == "articles already exist" {
					logger.Info("All articles are already fetched")
					offset = 0
					logger.Info("Format job completed, sleeping", "duration", FORMAT_INTERVAL, "next_offset", offset)
					time.Sleep(FORMAT_INTERVAL)
					continue
				}

				// Check if no URLs found
				if err.Error() == "no urls found" {
					// Check if this is the first attempt (offset=0) or we've been incrementing
					if offset == 0 {
						// Check feed statistics using driver function
						totalFeeds, processedFeeds, dbErr := driver.GetFeedStatistics(ctx, dbPool)
						if dbErr != nil {
							logger.Error("Failed to get feed statistics", "error", dbErr)
						} else if totalFeeds == 0 {
							logger.Info("No feeds found in database, waiting for feeds to be added")
						} else {
							logger.Info("No unprocessed feeds found", "total_feeds", totalFeeds, "processed_feeds", processedFeeds)
						}

						logger.Info("No URLs found at offset 0, sleeping and will retry later")
						time.Sleep(FORMAT_INTERVAL)
						continue
					} else {
						// We've been incrementing offset and reached the end, reset to 0
						logger.Info("Reached end of feeds, resetting offset to 0")
						offset = 0
						time.Sleep(FORMAT_INTERVAL)
						continue
					}
				}

				// Handle other errors
				logger.Error("Failed to run format job", "error", err)
				time.Sleep(30 * time.Second)
				continue
			}

			offset += OFFSET_STEP
			logger.Info("Format job completed, sleeping", "duration", FORMAT_INTERVAL, "next_offset", offset)
			time.Sleep(FORMAT_INTERVAL)
		}
	}()

	ch := make(chan error)
	go func() {
		ch <- healthCheckForNewsCreator()
	}()

	var offsetSummarize int

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
			logger.Info("Starting summarize job execution", "offset", offsetSummarize)
			foundArticles := job_for_summarize(offsetSummarize, ctx, dbPool)
			if !foundArticles {
				logger.Info("No articles found for summarization, resetting offset to 0")
				offsetSummarize = 0
			} else {
				offsetSummarize += OFFSET_STEP
			}
			logger.Info("Summarize job completed, sleeping", "duration", SUMMARIZE_INTERVAL, "next_offset", offsetSummarize)
			time.Sleep(SUMMARIZE_INTERVAL)
		}
	}()

	chQualityCheck := make(chan error)
	go func() {
		chQualityCheck <- healthCheckForNewsCreator()
	}()

	var offsetQualityCheck int
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Quality check job panicked", "panic", r)
			}
		}()

		// Wait for health check first
		err := <-chQualityCheck
		if err != nil {
			logger.Error("Quality check job is not healthy initially", "error", err)
			// Retry health check in a loop
			for {
				time.Sleep(30 * time.Second)
				err = healthCheckForNewsCreator()
				if err == nil {
					logger.Info("Quality check service is now healthy")
					break
				}
				logger.Error("Quality check service still not healthy", "error", err)
			}
		} else {
			logger.Info("Quality check service is healthy")
		}

		logger.Info("Quality check job started successfully")

		// Start quality check job loop
		for {
			logger.Info("Starting quality check job execution", "offset", offsetQualityCheck)
			foundArticles := job_for_quality_check(offsetQualityCheck, ctx, dbPool)
			if foundArticles == nil {
				logger.Info("No articles found for quality check, resetting offset to 0")
				offsetQualityCheck = 0
			} else {
				offsetQualityCheck += OFFSET_STEP
				logger.Info("Quality check job found articles", "count", len(foundArticles))
			}
			logger.Info("Quality check job completed, sleeping", "duration", "1 minute", "next_offset", offsetQualityCheck)
			time.Sleep(1 * time.Minute)
		}
	}()

	logger.Info("Starting pre-processor server on port 9200")

	// Add health check and debug endpoints
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"pre-processor"}`))
	})

	err = http.ListenAndServe(":9200", nil)
	if err != nil {
		logger.Error("Failed to start HTTP server", "error", err)
		panic(err)
	}
}

func job_for_format(offset int, ctx context.Context, dbPool *pgxpool.Pool) error {
	urls, err := driver.GetSourceURLs(offset, ctx, dbPool)
	if err != nil {
		logger.Logger.Error("Failed to get source URLs", "error", err)
		return errors.New("failed to get source URLs")
	}

	logger.Logger.Info("Source URLs", "urls length", len(urls), "offset", offset)

	if len(urls) == 0 {
		logger.Logger.Info("No source URLs found", "offset", offset)
		return errors.New("no urls found")
	}

	exists, err := driver.CheckArticleExists(ctx, dbPool, urls)
	if err != nil {
		logger.Logger.Error("Failed to check article exists", "error", err)
		return errors.New("failed to check article exists")
	}

	if exists {
		logger.Logger.Info("Articles already exist", "offset", offset)
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

		time.Sleep(5 * time.Second)
		logger.Logger.Info("Sleeping for 5 seconds. ", "index", i+1)
	}

	return nil
}

func job_for_summarize(offsetSummarize int, ctx context.Context, dbPool *pgxpool.Pool) bool {
	logger.Logger.Info("Starting summarize job", "offset", offsetSummarize)

	articles, err := driver.GetArticlesForSummarization(ctx, dbPool, offsetSummarize, OFFSET_STEP)
	if err != nil {
		logger.Logger.Error("Failed to get articles without summary", "error", err)
		return false
	}

	logger.Logger.Info("Found articles to summarize", "count", len(articles), "offset", offsetSummarize)

	if len(articles) == 0 {
		logger.Logger.Info("No articles found for summarization", "offset", offsetSummarize)
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

		logger.Logger.Info("Sleeping for 1 minute before next article", "currentIndex", i+1, "totalArticles", len(articles))
		time.Sleep(30 * time.Second)
	}

	logger.Logger.Info("Summarize job completed", "offset", offsetSummarize, "processedArticles", processedCount, "savedSummaries", savedCount)
	return true
}

func job_for_quality_check(offsetForScroing int, ctx context.Context, dbPool *pgxpool.Pool) []qualitychecker.ArticleWithScore {
	logger.Logger.Info("Starting quality check job", "offset", offsetForScroing)

	// Fetch articles and summaries
	articleWithScores, err := qualitychecker.FetchArticleAndSummaries(ctx, dbPool, offsetForScroing, OFFSET_STEP)
	if err != nil {
		logger.Logger.Error("Failed to fetch article and summary", "error", err)
		return nil
	}

	if len(articleWithScores) == 0 {
		logger.Logger.Info("No articles found for quality check", "offset", offsetForScroing)
		return nil
	}

	logger.Logger.Info("Found articles for quality check", "count", len(articleWithScores), "offset", offsetForScroing)

	// Process each article with quality scoring
	processedCount := 0
	successCount := 0
	errorCount := 0
	for i, articleWithScore := range articleWithScores {
		logger.Logger.Info("Processing article for quality check", "index", i, "articleID", articleWithScore.ArticleID)

		err = qualitychecker.RemoveLowScoreSummary(ctx, dbPool, &articleWithScore)
		if err != nil {
			logger.Logger.Error("Failed to process article quality check", "error", err, "articleID", articleWithScore.ArticleID)
			errorCount++
			continue
		}

		processedCount++
		successCount++
		logger.Logger.Info("Successfully processed article quality check", "articleID", articleWithScore.ArticleID)
		logger.Logger.Info("Sleeping for 10 seconds before next article", "currentIndex", i+1, "totalArticles", len(articleWithScores))
		time.Sleep(10 * time.Second)
	}

	logger.Logger.Info("Quality check completed",
		"processedArticles", processedCount,
		"successfulArticles", successCount,
		"errorArticles", errorCount,
		"totalArticles", len(articleWithScores),
		"offset", offsetForScroing)
	return articleWithScores
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
