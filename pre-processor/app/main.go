package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	articlefetcher "pre-processor/article-fetcher"
	"pre-processor/driver"
	"pre-processor/logger"
	"pre-processor/models"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const OFFSET_STEP = 40
const SUMMARIZE_INTERVAL = 20 * time.Second
const FORMAT_INTERVAL = 20 * time.Minute
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
			job_for_format(offset, ctx, dbPool)
			offset += OFFSET_STEP
			logger.Info("Format job completed, sleeping", "duration", FORMAT_INTERVAL, "next_offset", offset)
			time.Sleep(FORMAT_INTERVAL)
		}
	}()

	ch := make(chan error)
	go func() {
		ch <- healthCheckForNewsCreator()
	}()

	select {
	case err := <-ch:
		if err != nil {
			logger.Error("News creator is not healthy", "error", err)
			time.Sleep(30 * time.Second)
			ch <- healthCheckForNewsCreator()
		} else {
			logger.Info("News creator is healthy")
			// Run job in background. The job will run every 1 hour.
			var offsetSummarize int
			go func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("Summarize job panicked", "panic", r)
					}
				}()

				for {
					logger.Info("Starting summarize job execution", "offset", offsetSummarize)
					job_for_summarize(offsetSummarize, ctx, dbPool)
					offsetSummarize += OFFSET_STEP
					logger.Info("Summarize job completed, sleeping", "duration", SUMMARIZE_INTERVAL, "next_offset", offsetSummarize)
					time.Sleep(SUMMARIZE_INTERVAL)
				}
			}()
		}
	}

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

func job_for_format(offset int, ctx context.Context, dbPool *pgxpool.Pool) {
	urls, err := driver.GetSourceURLs(offset, ctx, dbPool)
	if err != nil {
		logger.Logger.Error("Failed to get source URLs", "error", err)
		return
	}

	logger.Logger.Info("Source URLs", "urls", urls)

	for i, url := range urls {
		logger.Logger.Info("Fetching article", "url", url.String(), "index", i)
		article, err := articlefetcher.FetchArticle(url)
		if err != nil {
			logger.Logger.Error("Failed to fetch article", "error", err)
			continue
		}

		// Insert article to database immediately after fetching
		err = driver.CreateArticle(ctx, dbPool, article)
		if err != nil {
			logger.Logger.Error("Failed to create article", "error", err)
		} else {
			logger.Logger.Info("Successfully created article", "articleID", article.ID, "title", article.Title)
		}

		time.Sleep(5 * time.Second)
		logger.Logger.Info("Sleeping for 5 seconds. ", "index", i+1)
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
		time.Sleep(30 * time.Second)
	}

	logger.Logger.Info("Summarize job completed", "offset", offsetSummarize, "processedArticles", processedCount, "savedSummaries", savedCount)
}

func healthCheckForNewsCreator() error {
	payload := map[string]interface{}{
		"model":  MODEL_ID,
		"prompt": "Say hello!",
		"stream": false,
		"options": map[string]interface{}{
			"temperature":    0.3,
			"top_p":          0.8,
			"num_predict":    1500,
			"repeat_penalty": 1.1,
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Logger.Error("Failed to marshal payload", "error", err)
		return err
	}

	resp, err := http.Post("http://news-creator:11434/api/generate", "application/json", bytes.NewBuffer(jsonData))
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
