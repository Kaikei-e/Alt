package main

import (
	"context"
	"net/http"
	articlefetcher "pre-processor/article-fetcher"
	"pre-processor/logger"
	"pre-processor/models"
	"pre-processor/repository"
	"time"

	"github.com/jackc/pgx/v5"
)

const OFFSET_STEP = 20

func main() {
	logger := logger.Init()

	ctx := context.Background()
	db, err := repository.Init(ctx)
	if err != nil {
		logger.Error("Failed to initialize database", "error", err)
		panic(err)
	}

	// Run job in background. The job will run every 1 hour.
	var offset int
	go func() {
		for {
			job(offset, ctx, db)
			offset += OFFSET_STEP
			time.Sleep(30 * time.Minute)
		}
	}()

	logger.Info("Starting pre-processor server on port 9200")
	http.ListenAndServe(":9200", nil)
}

func job(offset int, ctx context.Context, db *pgx.Conn) {
	urls, err := repository.GetSourceURLs(offset, ctx, db)
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
		err = repository.CreateArticle(ctx, db, article)
		if err != nil {
			logger.Logger.Error("Failed to create article", "error", err)
			continue
		}
	}
}
