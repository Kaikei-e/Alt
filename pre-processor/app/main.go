package main

import (
	"context"
	"net/http"
	articlefetcher "pre-processor/article-fetcher"
	"pre-processor/driver"
	"pre-processor/logger"
	"pre-processor/models"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const OFFSET_STEP = 20
const SUMMARIZE_INTERVAL = 1 * time.Hour
const FORMAT_INTERVAL = 30 * time.Minute

func main() {
	logger := logger.Init()

	ctx := context.Background()
	dbPool, err := driver.Init(ctx)
	if err != nil {
		logger.Error("Failed to initialize database", "error", err)
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
			job_for_format(offset, ctx, dbPool)
			offset += OFFSET_STEP
			logger.Info("Format job completed, sleeping", "duration", FORMAT_INTERVAL, "next_offset", offset)
			time.Sleep(FORMAT_INTERVAL)
		}
	}()

	// Run job in background. The job will run every 1 hour.
	var offsetSummarize int
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Summarize job panicked", "panic", r)
			}
		}()

		// Add a delay to stagger job execution and avoid database connection conflicts
		logger.Info("Summarize job: waiting 30 seconds to stagger execution")
		time.Sleep(30 * time.Second)
		logger.Info("Summarize job: delay complete, starting goroutine")

		logger.Info("Starting summarize job goroutine")
		for {
			logger.Info("Starting summarize job execution", "offset", offsetSummarize)
			job_for_summarize(offsetSummarize, ctx, dbPool)
			offsetSummarize += OFFSET_STEP
			logger.Info("Summarize job completed, sleeping", "duration", SUMMARIZE_INTERVAL, "next_offset", offsetSummarize)
			time.Sleep(SUMMARIZE_INTERVAL)
		}
	}()

	logger.Info("Starting pre-processor server on port 9200")

	// Add health check and debug endpoints
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"pre-processor"}`))
	})

	// Manual trigger endpoints for testing
	http.HandleFunc("/trigger/format", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		logger.Info("Manual trigger: format job")
		go job_for_format(offset, ctx, dbPool)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"triggered","job":"format"}`))
	})

	http.HandleFunc("/trigger/summarize", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		logger.Info("Manual trigger: summarize job")
		go job_for_summarize(offsetSummarize, ctx, dbPool)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"triggered","job":"summarize"}`))
	})

	err = http.ListenAndServe(":9200", nil)
	if err != nil {
		logger.Error("Failed to start HTTP server", "error", err)
		panic(err)
	}
}

func job_for_format(offset int, ctx context.Context, dbPool *pgxpool.Pool) {
	urls, err := driver.GetSourceURLs(offset, ctx, dbPool)
	if err != nil {
		logger.Logger.Error("Failed to get source URLs", "error", err)
		return
	}

	logger.Logger.Info("Source URLs", "urls", urls)

	var articles []*models.Article
	for i, url := range urls {
		logger.Logger.Info("Fetching article", "url", url.String(), "index", i)
		article, err := articlefetcher.FetchArticle(url)
		if err != nil {
			logger.Logger.Error("Failed to fetch article", "error", err)
			continue
		}

		articles = append(articles, article)
		time.Sleep(5 * time.Second)
		logger.Logger.Info("Sleeping for 5 seconds. ", "index", i+1)
	}

	for _, article := range articles {
		err = driver.CreateArticle(ctx, dbPool, article)
		if err != nil {
			logger.Logger.Error("Failed to create article", "error", err)
			continue
		}
	}
}

func job_for_summarize(offsetSummarize int, ctx context.Context, dbPool *pgxpool.Pool) {
	logger.Logger.Info("Starting summarize job", "offset", offsetSummarize)

	articles, err := driver.GetArticlesForSummarization(ctx, dbPool, offsetSummarize, OFFSET_STEP)
	if err != nil {
		logger.Logger.Error("Failed to get articles without summary", "error", err)
		return
	}

	logger.Logger.Info("Found articles to summarize", "count", len(articles), "offset", offsetSummarize)

	if len(articles) == 0 {
		logger.Logger.Info("No articles found for summarization", "offset", offsetSummarize)
		return
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
		time.Sleep(1 * time.Minute)
	}

	logger.Logger.Info("Summarize job completed", "offset", offsetSummarize, "processedArticles", processedCount, "savedSummaries", savedCount)
}
